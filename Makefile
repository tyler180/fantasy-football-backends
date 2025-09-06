# ===== Config =====
GOOS   ?= linux
GOARCH ?= amd64
LAMBDA_MOD_DIR := tools/pfr-weekly
LAMBDA_MAIN_PKG := ./cmd/pfr-weekly
TF_DIR := infra/terraform

BINARY := bootstrap
ZIP    := $(TF_DIR)/artifacts/pfr-weekly.zip

.PHONY: build-lambda zip-lambda clean tf-init tf-plan tf-apply tf-destroy test

test:
	go test -C tools/pfr-weekly ./...

build-lambda:
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=0 \
		go build -C $(LAMBDA_MOD_DIR) -o ../../$(BINARY) $(LAMBDA_MAIN_PKG)
	chmod +x $(BINARY)

zip-lambda: build-lambda
	mkdir -p $(TF_DIR)/artifacts
	cd . && zip -9 $(ZIP) $(BINARY)
	rm -f $(BINARY)

clean:
	rm -f $(BINARY) $(ZIP)

tf-init:
	terraform -chdir=$(TF_DIR) init

tf-plan:
	terraform -chdir=$(TF_DIR) plan

tf-apply:
	terraform -chdir=$(TF_DIR) apply

SEASON ?= 2024

tf-apply:
	terraform -chdir=$(TF_DIR) apply -var="season=$(SEASON)"