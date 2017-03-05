package dynamodb

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	godynamodb "github.com/aws/aws-sdk-go/service/dynamodb"
	godynamodbiface "github.com/aws/aws-sdk-go/service/dynamodb/dynamodbiface"
	"github.com/pkg/errors"

	"github.com/yuuki/diamondb/pkg/config"
	"github.com/yuuki/diamondb/pkg/mathutil"
	"github.com/yuuki/diamondb/pkg/model"
	"github.com/yuuki/diamondb/pkg/storage/util"
)

//go:generate mockgen -source ../../../vendor/github.com/aws/aws-sdk-go/service/dynamodb/dynamodbiface/interface.go -destination dynamodb_mock.go -package dynamodb

// ReadWriter defines the interface for DynamoDB reader and writer.
type ReadWriter interface {
	Ping() error
	Client() godynamodbiface.DynamoDBAPI
	CreateTable(*CreateTableParam) error
	Fetch(string, time.Time, time.Time) (model.SeriesMap, error)
	batchGet(q *query) (model.SeriesMap, error)
	Put(string, string, int64, int64, map[int64]float64) error
}

// DynamoDB provides a dynamodb client.
type DynamoDB struct {
	svc godynamodbiface.DynamoDBAPI
}

type timeSlot struct {
	tableName string
	itemEpoch int64
}

type query struct {
	names []string
	start time.Time
	end   time.Time
	slot  *timeSlot
	step  int
	// context
}

const (
	oneYear time.Duration = time.Duration(24*360) * time.Hour
	oneWeek time.Duration = time.Duration(24*7) * time.Hour
	oneDay  time.Duration = time.Duration(24*1) * time.Hour
)

var (
	dynamodbBatchLimit = 100

	oneYearSeconds = int(oneYear.Seconds())
	oneWeekSeconds = int(oneWeek.Seconds())
	oneDaySeconds  = int(oneDay.Seconds())
)

var _ ReadWriter = &DynamoDB{}

// New creates a new DynamoDB.
func New() (*DynamoDB, error) {
	awsConf := aws.NewConfig().WithRegion(config.Config.DynamoDBRegion)
	if config.Config.DynamoDBEndpoint != "" {
		// For dynamodb-local configuration
		awsConf.WithEndpoint(config.Config.DynamoDBEndpoint)
		awsConf.WithCredentials(credentials.NewStaticCredentials("dummy", "dummy", "dummy"))
	}
	sess, err := session.NewSession(awsConf)
	if err != nil {
		return nil, errors.Wrapf(err,
			"failed to create session for dynamodb (%s,%s)",
			config.Config.DynamoDBRegion,
			config.Config.DynamoDBEndpoint,
		)
	}
	return &DynamoDB{
		svc: godynamodb.New(sess),
	}, nil
}

// Client returns the DynamoDB client.
func (d *DynamoDB) Client() godynamodbiface.DynamoDBAPI {
	return d.svc
}

// Ping pings DynamoDB endpoint.
func (d *DynamoDB) Ping() error {
	var params *godynamodb.DescribeLimitsInput
	_, err := d.svc.DescribeLimits(params)
	if err != nil {
		return errors.Wrapf(err, "failed to ping dynamodb")
	}
	return nil
}

// CreateTableParam is parameter set of CreateTable.
type CreateTableParam struct {
	Name string
	RCU  int64 // ReadCapacityUnits
	WCU  int64 // WriteCapacityUnits
}

// CreareTable creates a dynamodb table to store time series data.
func (d *DynamoDB) CreateTable(param *CreateTableParam) error {
	_, err := d.svc.CreateTable(&godynamodb.CreateTableInput{
		TableName: aws.String(param.Name),
		AttributeDefinitions: []*godynamodb.AttributeDefinition{
			{
				AttributeName: aws.String("Name"),
				AttributeType: aws.String(godynamodb.ScalarAttributeTypeS),
			},
			{
				AttributeName: aws.String("Timestamp"),
				AttributeType: aws.String(godynamodb.ScalarAttributeTypeN),
			},
		},
		KeySchema: []*godynamodb.KeySchemaElement{
			{
				AttributeName: aws.String("Name"),
				KeyType:       aws.String("HASH"),
			},
			{
				AttributeName: aws.String("Timestamp"),
				KeyType:       aws.String("RANGE"),
			},
		},
		ProvisionedThroughput: &godynamodb.ProvisionedThroughput{
			ReadCapacityUnits:  aws.Int64(param.RCU),
			WriteCapacityUnits: aws.Int64(param.WCU),
		},
		// TODO StreamSpecification to export to s3
	})
	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok {
			if awsErr.Code() == "ResourceInUseException" {
				// Skip if the table already exists
				return nil
			}
		}
		return errors.Wrapf(err, "failed to create dynamodb table (%s,%d,%d)",
			param.Name, param.RCU, param.WCU)
	}

	err = d.svc.WaitUntilTableExists(&godynamodb.DescribeTableInput{
		TableName: aws.String(param.Name),
	})
	if err != nil {
		return errors.Wrapf(err, "failed to wait until table exists (%s,%d,%d)",
			param.Name, param.RCU, param.WCU)
	}

	_, err = d.svc.UpdateTimeToLive(&godynamodb.UpdateTimeToLiveInput{
		TableName: aws.String(param.Name),
		TimeToLiveSpecification: &godynamodb.TimeToLiveSpecification{
			AttributeName: aws.String("TTL"),
			Enabled:       aws.Bool(true),
		},
	})
	if err != nil {
		return errors.Wrapf(err, "failed to set TTL to (%s,%d,%d)",
			param.Name, param.RCU, param.WCU)
	}
	return nil
}

// Fetch fetches datapoints by name from start until end.
func (d *DynamoDB) Fetch(name string, start, end time.Time) (model.SeriesMap, error) {
	slots, step := selectTimeSlots(start, end, config.Config.DynamoDBTablePrefix)
	nameGroups := util.GroupNames(util.SplitName(name), dynamodbBatchLimit)
	numQueries := len(slots) * len(nameGroups)

	type result struct {
		value model.SeriesMap
		err   error
	}
	c := make(chan *result, numQueries)
	for _, slot := range slots {
		for _, names := range nameGroups {
			q := &query{
				names: names,
				start: start,
				end:   end,
				slot:  slot,
				step:  step,
			}
			go func(q *query) {
				sm, err := d.batchGet(q)
				c <- &result{value: sm, err: err}
			}(q)
		}
	}
	sm := make(model.SeriesMap, len(nameGroups))
	for i := 0; i < numQueries; i++ {
		ret := <-c
		if ret.err != nil {
			return nil, ret.err
		}
		sm.MergePointsToMap(ret.value)
	}
	return sm, nil
}

func batchGetResultToMap(resp *godynamodb.BatchGetItemOutput, q *query) model.SeriesMap {
	sm := make(model.SeriesMap, len(resp.Responses))
	for _, xs := range resp.Responses {
		for _, x := range xs {
			name := (*x["MetricName"].S)
			points := make(model.DataPoints, 0, len(x["Values"].BS))
			for _, y := range x["Values"].BS {
				t := int64(binary.BigEndian.Uint64(y[0:8]))
				v := math.Float64frombits(binary.BigEndian.Uint64(y[8:]))
				// Trim datapoints out of [start, end]
				if t < q.start.Unix() || q.end.Unix() < t {
					continue
				}
				points = append(points, model.NewDataPoint(t, v))
			}
			sm[name] = model.NewSeriesPoint(name, points, q.step)
		}
	}
	return sm
}

func (d *DynamoDB) batchGet(q *query) (model.SeriesMap, error) {
	var keys []map[string]*godynamodb.AttributeValue
	for _, name := range q.names {
		keys = append(keys, map[string]*godynamodb.AttributeValue{
			"MetricName": {S: aws.String(name)},
			"Timestamp":  {N: aws.String(fmt.Sprintf("%d", q.slot.itemEpoch))},
		})
	}
	items := make(map[string]*godynamodb.KeysAndAttributes)
	items[q.slot.tableName] = &godynamodb.KeysAndAttributes{Keys: keys}
	params := &godynamodb.BatchGetItemInput{
		RequestItems:           items,
		ReturnConsumedCapacity: aws.String("NONE"),
	}
	resp, err := d.svc.BatchGetItem(params)
	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok {
			if awsErr.Code() == "ResourceNotFoundException" {
				// Don't handle ResourceNotFoundException as error
				// bacause diamondb web return length 0 series as 200.
				return model.SeriesMap{}, nil
			}
		}
		return nil, errors.Wrapf(err,
			"failed to call dynamodb API batchGetItem (%s,%d,%d)",
			q.slot.tableName, q.slot.itemEpoch, q.step,
		)
	}
	return batchGetResultToMap(resp, q), nil
}

func (d *DynamoDB) Put(name, retention string, tableEpoch, itemEpoch int64, tv map[int64]float64) error {
	tableName := fmt.Sprintf("%s-%s-%d", config.Config.DynamoDBTablePrefix, retention, tableEpoch)

	vals := make([][]byte, 0, len(tv))
	for timestamp, value := range tv {
		buf := new(bytes.Buffer)
		binary.Write(buf, binary.BigEndian, timestamp)
		binary.Write(buf, binary.BigEndian, math.Float64bits(value))
		vals = append(vals, buf.Bytes())
	}
	params := &godynamodb.UpdateItemInput{
		TableName: aws.String(tableName),
		Key: map[string]*godynamodb.AttributeValue{
			"MetricName": {S: aws.String(name)},
			"Timestamp":  {N: aws.String(fmt.Sprintf("%d", itemEpoch))},
		},
		UpdateExpression: aws.String("ADD #values_set :new_values"),
		ExpressionAttributeNames: map[string]*string{
			"#values_set": aws.String("Values"),
		},
		ExpressionAttributeValues: map[string]*godynamodb.AttributeValue{
			":new_values": {BS: vals},
		},
		ReturnValues: aws.String("NONE"),
	}
	if _, err := d.svc.UpdateItem(params); err != nil {
		return errors.Wrapf(err, "failed to call dynamodb API putItem (%s,%s,%d)",
			tableName, name, itemEpoch)
	}
	return nil
}

func selectTimeSlots(startTime, endTime time.Time, tablePrefix string) ([]*timeSlot, int) {
	var (
		tableName      string
		step           int
		tableEpochStep int
		itemEpochStep  int
	)
	diff := endTime.Sub(startTime)
	switch {
	case oneYear <= diff:
		tableName = tablePrefix + "-1d1y"
		tableEpochStep = oneYearSeconds
		itemEpochStep = tableEpochStep
		step = 60 * 60 * 24
	case oneWeek <= diff:
		tableName = tablePrefix + "-1h7d"
		tableEpochStep = 60 * 60 * 24 * 7
		itemEpochStep = tableEpochStep
		step = 60 * 60
	case oneDay <= diff:
		tableName = tablePrefix + "-5m1d"
		tableEpochStep = 60 * 60 * 24
		itemEpochStep = tableEpochStep
		step = 5 * 60
	default:
		tableName = tablePrefix + "-1m1h"
		tableEpochStep = 60 * 60 * 24
		itemEpochStep = 60 * 60
		step = 60
	}

	slots := []*timeSlot{}
	startTableEpoch := startTime.Unix() - startTime.Unix()%int64(tableEpochStep)
	endTableEpoch := endTime.Unix()
	for tableEpoch := startTableEpoch; tableEpoch < endTableEpoch; tableEpoch += int64(tableEpochStep) {
		startItemEpoch := mathutil.MaxInt64(tableEpoch, startTime.Unix()-startTime.Unix()%int64(itemEpochStep))
		endItemEpoch := mathutil.MinInt64(tableEpoch+int64(tableEpochStep), endTime.Unix())
		for itemEpoch := startItemEpoch; itemEpoch < endItemEpoch; itemEpoch += int64(itemEpochStep) {
			slot := timeSlot{
				tableName: fmt.Sprintf("%s-%d", tableName, tableEpoch),
				itemEpoch: itemEpoch,
			}
			slots = append(slots, &slot)
		}
	}

	return slots, step
}
