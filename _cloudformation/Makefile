all: infrastrcuture/create storage/create app/create info

info:
	@echo "diamondb URL:" $(shell aws cloudformation describe-stacks --stack-name diamondb-app | jq -r -M '.Stacks[0].Outputs | map(select(.OutputKey == "DiamonDBUrl"))[0].OutputValue')

# ------------------------------------------------------

.PHONY: infrastructure/create
infrastructure/create:
	pushd ./infrastructure && make create && popd

.PHONY: infrastructure/update
infrastructure/update:
	pushd ./infrastructure && make update && popd

.PHONY: infrastructure/diff
infrastructure/diff:
	pushd ./infrastructure && make diff && popd

.PHONY: infrastructure/apply
infrastructure/apply:
	pushd ./infrastructure && make apply && popd

# ------------------------------------------------------

.PHONY: storage/create
storage/create:
	pushd ./storage && make create && popd

.PHONY: storage/update
storage/update:
	pushd ./storage && make update && popd

.PHONY: storage/diff
storage/diff:
	pushd ./storage && make diff && popd

.PHONY: storage/apply
storage/apply:
	pushd ./storage && make apply && popd

# ------------------------------------------------------

.PHONY: app/create
app/create:
	pushd ./app && make create && popd

.PHONY: app/update
app/update:
	pushd ./app && make update && popd

.PHONY: app/diff
app/diff:
	pushd ./app && make diff && popd

.PHONY: app/apply
app/apply:
	pushd ./app && make apply && popd
