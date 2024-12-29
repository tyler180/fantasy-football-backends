#!/bin/bash

# Variables
LAMBDA_NAME="lambda.zip"                     # Name of the output ZIP file
GO_FILE_DIR="sports-ref-scraper/cmd"         # Directory containing main.go, go.mod, and go.sum
BINARY_NAME="bootstrap"                      # Name of the compiled binary (must be 'bootstrap')
BUILD_DIR=$(pwd)                             # Current working directory
OUTPUT_DIR="$BUILD_DIR"                      # Directory to store the final lambda.zip
LAMBDA_FUNCTION_NAME="player_scraper_lambda" # Name of the AWS Lambda function

# Function to print status
print_status() {
  echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1"
}

# Step 1: Check if Go is installed
if ! command -v go &> /dev/null; then
  print_status "Go is not installed. Please install Go and try again."
  exit 1
fi

# Step 2: Verify the Go files exist
if [ ! -f "$GO_FILE_DIR/main.go" ] || [ ! -f "$GO_FILE_DIR/go.mod" ]; then
  print_status "Required files not found in $GO_FILE_DIR. Ensure main.go, go.mod, and go.sum are in this directory."
  exit 1
fi

# Step 3: Clean up previous build artifacts
print_status "Cleaning up old build artifacts..."
rm -f "$OUTPUT_DIR/$BINARY_NAME" "$OUTPUT_DIR/$LAMBDA_NAME"

# Step 4: Build the Go binary for AWS Lambda (Amazon Linux 2023 runtime)
print_status "Building the Go binary for AWS Lambda (AL2023)..."
cd "$GO_FILE_DIR" || exit
GOOS=linux GOARCH=amd64 go build -o "$OUTPUT_DIR/$BINARY_NAME" ./main.go
if [ $? -ne 0 ]; then
  print_status "Failed to build the Go binary."
  exit 1
fi
cd "$BUILD_DIR" || exit
print_status "Build completed: $OUTPUT_DIR/$BINARY_NAME"

# Step 5: Package the binary into a lambda.zip file
print_status "Packaging the binary into $LAMBDA_NAME..."
zip -j "$OUTPUT_DIR/$LAMBDA_NAME" "$OUTPUT_DIR/$BINARY_NAME" > /dev/null
if [ $? -ne 0 ]; then
  print_status "Failed to create the ZIP file."
  exit 1
fi
print_status "Package completed: $OUTPUT_DIR/$LAMBDA_NAME"

# Step 6: Verify the ZIP file
print_status "Verifying the ZIP file..."
if unzip -l "$OUTPUT_DIR/$LAMBDA_NAME" | grep -q "$BINARY_NAME"; then
  print_status "The ZIP file contains the compiled binary."
else
  print_status "The ZIP file does not contain the binary. Please check."
  exit 1
fi

# Step 7: Upload the new build to AWS Lambda
print_status "Uploading $LAMBDA_NAME to Lambda function $LAMBDA_FUNCTION_NAME..."
aws lambda update-function-code --function-name "$LAMBDA_FUNCTION_NAME" --zip-file fileb://"$OUTPUT_DIR/$LAMBDA_NAME" | jq
if [ $? -ne 0 ]; then
  print_status "Failed to upload $LAMBDA_NAME to Lambda."
  exit 1
fi
print_status "Successfully uploaded $LAMBDA_NAME to Lambda function $LAMBDA_FUNCTION_NAME."

# Final success message
print_status "Build, packaging, and deployment completed successfully!"
exit 0