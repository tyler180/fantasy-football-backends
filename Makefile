# Repo layout (root module):
#   internal/...
#   tools/pfr-weekly/cmd/pfr-weekly/main.go
#   tools/pfr-snaps/cmd/pfr-snaps/main.go
#   infra/terraform/...
#   artifacts/ (built by this Makefile)

# ---- Config ----
REGION               ?= us-west-2
ARCH                 ?= amd64           # set to arm64 if your Lambda uses arm64
LAMBDA_WEEKLY_NAME   ?= pfr-weekly-2024
LAMBDA_SNAPS_NAME    ?= pfr-snaps-2024

ARTIFACTS_DIR        := artifacts
BOOTSTRAP_WEEKLY     := $(ARTIFACTS_DIR)/bootstrap-weekly
BOOTSTRAP_SNAPS      := $(ARTIFACTS_DIR)/bootstrap-snaps
ZIP_WEEKLY           := $(ARTIFACTS_DIR)/pfr-weekly.zip
ZIP_SNAPS            := $(ARTIFACTS_DIR)/pfr-snaps.zip
ZIP_DIR 			 := infra/artifacts

# ---- Helpers ----
.PHONY: all deps tidy clean \
        build-weekly zip-weekly deploy-weekly \
        build-snaps zip-snaps deploy-snaps \
        tf-init tf-plan tf-apply \
		zip-athena-materializer

all: zip-weekly zip-snaps

deps:
	@go version
	@which zip >/dev/null || (echo "Please install 'zip' CLI" && exit 1)
	@mkdir -p $(ARTIFACTS_DIR)

tidy:
	go mod tidy

clean:
	rm -f $(BOOTSTRAP_WEEKLY) $(BOOTSTRAP_SNAPS) $(ZIP_WEEKLY) $(ZIP_SNAPS)

# ---- pfr-weekly (roster + materialize defense) ----
build-weekly: deps tidy
	GOOS=linux GOARCH=$(ARCH) CGO_ENABLED=0 \
		go build -o $(BOOTSTRAP_WEEKLY) ./tools/pfr-weekly/cmd/pfr-weekly

zip-weekly: build-weekly
	# Lambda (provided.al2023) expects a file named 'bootstrap' at the zip root
	cd $(ARTIFACTS_DIR) && cp bootstrap-weekly bootstrap && zip -9 pfr-weekly.zip bootstrap && rm -f bootstrap
	@echo "Wrote $(ZIP_WEEKLY)"

deploy-weekly: zip-weekly
	aws lambda update-function-code \
	  --region $(REGION) \
	  --function-name $(LAMBDA_WEEKLY_NAME) \
	  --zip-file fileb://$(ZIP_WEEKLY)

# ---- pfr-snaps (per-game snap% + trends) ----
build-snaps: deps tidy
	GOOS=linux GOARCH=$(ARCH) CGO_ENABLED=0 \
		go build -o $(BOOTSTRAP_SNAPS) ./tools/pfr-snaps/cmd/pfr-snaps

zip-snaps: build-snaps
	cd $(ARTIFACTS_DIR) && cp bootstrap-snaps bootstrap && zip -9 pfr-snaps.zip bootstrap && rm -f bootstrap
	@echo "Wrote $(ZIP_SNAPS)"

deploy-snaps: zip-snaps
	aws lambda update-function-code \
	  --region $(REGION) \
	  --function-name $(LAMBDA_SNAPS_NAME) \
	  --zip-file fileb://$(ZIP_SNAPS)

.PHONY: zip-nflverse-curator
zip-nflverse-curator:
	mkdir -p $(ZIP_DIR)
	cd tools/nflverse-curator && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o ../../infra/artifacts/bootstrap ./cmd/nflverse-curator
	cd infra/artifacts && rm -f nflverse-curator.zip && zip -9 nflverse-curator.zip bootstrap && rm -f bootstrap

.PHONY: zip-athena-materializer
zip-athena-materializer:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o ./infra/artifacts/bootstrap ./tools/athena-materializer/cmd/athena-materializer \
	cd ../../infra/artifacts && zip -9 athena-materializer.zip bootstrap && rm -f bootstrap

deploy-athena-materializer: zip-athena-materializer
	aws lambda update-function-code --region us-west-2 --function-name athena-materializer --zip-file fileb://infra/artifacts/athena-materializer.zip

athena-materializer-test:
	aws lambda invoke --region us-west-2 --function-name athena-materializer --cli-binary-format raw-in-base64-out --payload '{}' out_materializer.json --log-type Tail --query 'LogResult' --output text | base64 --decode

# ---- Terraform (infra/terraform) ----
tf-init:
	cd infra/terraform && terraform init

tf-plan:
	cd infra/terraform && terraform plan

tf-apply:
	cd infra/terraform && terraform apply -auto-approve