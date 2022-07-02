.PHONY: help
.DEFAULT_GOAL := help
SHELL=bash

dc_path=./docker-compose.yaml
dev_clickhouse_server_container=dev_clickhouse_server
dev_front_basicrum_go_container=dev_front_basicrum_go

dropSql   := $(shell cat ./dev/recycle/1_drop_tables.sql)
createSql := $(shell cat ./dev/recycle/2_create_table.sql)

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

up: ## Starts the environment
	docker-compose -f ${dc_path} build
	docker-compose -f ${dc_path} up -d

down: ## Stops the environment
	docker-compose -f ${dc_path} down

restart: down up # Restart the environment

rebuild: ## Rebuilds the environment from scratch
	@/bin/echo -n "All the volumes will be deleted. You will loose data in DB. Are you sure? [y/N]: " && read answer && \
	[[ $${answer:-N} = y ]] && make destroy

destroy: ## Destroys thel environment
	docker-compose -f ${dc_path} down --volumes --remove-orphans
	docker-compose -f ${dc_path} rm -vsf

jump_front_basicrum: ## Jump to the front_basicrum container
	docker-compose -f ${dc_path} exec ${dev_front_basicrum_go_container} bash

logs_front_basicrum: ## Log messages of the front_basicrum container
	docker-compose -f ${dc_path} logs ${dev_front_basicrum_go_container}

restart_front_basicrum: ## Restart the front_basicrum container
	docker-compose -f ${dc_path} restart ${dev_front_basicrum_go_container}

jump_clickhouse_server: ## Jump to the clickhouse_server container
	docker-compose -f ${dc_path} exec ${dev_clickhouse_server_container} bash

run_integration_tests: ## Run Integration Tests
	docker-compose -f ${dc_path} exec -w /go/src/github.com/basicrum/front_basicrum_go/it ${dev_front_basicrum_go_container} go test

recycle_tables: ## Recreating the ClickHouse tables
	docker-compose -f ${dc_path} exec -T ${dev_clickhouse_server_container} clickhouse-client --host 127.0.0.1 -q "${dropSql}"
	docker-compose -f ${dc_path} exec -T ${dev_clickhouse_server_container} clickhouse-client --host 127.0.0.1 -q "${createSql}"

