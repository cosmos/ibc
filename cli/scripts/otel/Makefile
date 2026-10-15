# SPDX-License-Identifier: Apache-2.0

up:
	docker compose up -d
	@echo "Grafana: http://127.0.0.1:3001"

down:
	docker compose down

dashboard-export:
	bash dashboard.sh export

dashboard-import:
	bash dashboard.sh import $(if $(filter 1,$(OVERWRITE)),--overwrite)

.PHONY: up down dashboard-export dashboard-import
