package tsdb

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/golang/mock/gomock"

	"github.com/yuuki/diamondb/lib/model"
)

type MockDynamoDBParam struct {
	TableName string
	ItemEpoch int64
	Metrics   []*model.Metric
}

func SetMockDynamoDB(t *testing.T, m *MockDynamoDBParam) *gomock.Controller {
	ctrl := gomock.NewController(t)
	dmock := NewMockDynamoDBAPI(ctrl)

	var keys []map[string]*dynamodb.AttributeValue
	for _, metric := range m.Metrics {
		keys = append(keys, map[string]*dynamodb.AttributeValue{
			"MetricName": {S: aws.String(metric.Name)},
			"Timestamp":  {N: aws.String(fmt.Sprintf("%d", m.ItemEpoch))},
		})
	}
	items := make(map[string]*dynamodb.KeysAndAttributes)
	items[m.TableName] = &dynamodb.KeysAndAttributes{Keys: keys}
	params := &dynamodb.BatchGetItemInput{
		RequestItems:           items,
		ReturnConsumedCapacity: aws.String("NONE"),
	}

	expect := dmock.EXPECT().BatchGetItem(params)

	responses := make(map[string][]map[string]*dynamodb.AttributeValue)
	for _, metric := range m.Metrics {
		var vals [][]byte
		for _, point := range metric.DataPoints {
			buf := new(bytes.Buffer)
			binary.Write(buf, binary.BigEndian, int64(point.Timestamp))
			binary.Write(buf, binary.BigEndian, math.Float64bits(point.Value))
			vals = append(vals, buf.Bytes())
		}
		attribute := map[string]*dynamodb.AttributeValue{
			"MetricName": {S: aws.String(metric.Name)},
			"Timestamp":  {N: aws.String(fmt.Sprintf("%d", m.ItemEpoch))},
			"Values":     {BS: vals},
		}
		responses[m.TableName] = append(responses[m.TableName], attribute)
	}

	expect.Return(&dynamodb.BatchGetItemOutput{
		Responses: responses,
	}, nil)
	SetDynamoDB(dmock)

	return ctrl
}
