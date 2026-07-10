SOURCE_DIRS := ./cmd ./internal

.PHONY: vendor tidy fmt fmt-check lint test build all journal web web-check

vendor:
	go mod vendor

tidy:
	go mod tidy && git diff --exit-code go.mod go.sum

fmt:
	goimports -w $(SOURCE_DIRS)

fmt-check:
	@test -z "$$(gofmt -l -s $(SOURCE_DIRS))" || (gofmt -l -s $(SOURCE_DIRS); exit 1)

lint:
	go vet ./... && golangci-lint run -c .golangci.yml

test:
	go test -mod=vendor -race -covermode=atomic -coverprofile=coverage.out ./...

build:
	go build -mod=vendor -trimpath -ldflags "-s -w -buildid=" -o bin/ ./cmd/...

all: vendor tidy fmt-check lint test build

# Compile the TypeScript sources in web-src/ into web/assets/js (compiled JS
# is committed so the site deploys with no build step).
web:
	npm ci --prefix web-src
	npm run --prefix web-src build

web-check: web
	git diff --exit-code -- web/assets/js

# Re-export the canonical weekly price journals from the cto-agent DB (see data/journal/README.md).
# cmc_id 1274 = WAVES; 1765 = EOS, whose series continues as Vaulta (A) after the rebrand.
journal:
	echo "week_end,price_avg_usd" > data/journal/waves.csv
	sqlite3 -readonly -csv ../../aggelion/cto-agent/data/cmc_history.db \
	  "SELECT substr(week_end,1,4)||'-'||substr(week_end,5,2)||'-'||substr(week_end,7,2) AS week_end, price_avg FROM price_weekly WHERE cmc_id=1274 ORDER BY week_end" \
	  >> data/journal/waves.csv
	echo "week_end,price_avg_usd" > data/journal/eos.csv
	sqlite3 -readonly -csv ../../aggelion/cto-agent/data/cmc_history.db \
	  "SELECT substr(week_end,1,4)||'-'||substr(week_end,5,2)||'-'||substr(week_end,7,2) AS week_end, price_avg FROM price_weekly WHERE cmc_id=1765 ORDER BY week_end" \
	  >> data/journal/eos.csv
