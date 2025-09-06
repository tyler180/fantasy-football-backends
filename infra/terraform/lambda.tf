##########################
# Lambda (Go)
##########################

# Build your binary as "main" and zip it:
# GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o main ./cmd/pfr-weekly
# zip -9 pfr-weekly.zip main

resource "aws_lambda_function" "pfr_weekly" {
  function_name    = "pfr-weekly-${var.season}"
  filename         = "${path.module}/artifacts/pfr-weekly.zip"
  source_code_hash = filebase64sha256("${path.module}/artifacts/pfr-weekly.zip")
  handler          = "bootstrap"
  runtime          = "provided.al2023"
  architectures    = ["x86_64"]
  timeout          = 60
  memory_size      = 512
  role             = aws_iam_role.pfr_weekly.arn

  environment {
    variables = {
      TABLE_NAME = aws_dynamodb_table.players.name
      SEASON     = var.season
      MAX_AGE    = "24"
      POSITIONS  = "DE,DT,NT,DL,EDGE,LB,ILB,OLB,MLB,CB,DB,S,FS,SS,SAF,NB"
      S3_BUCKET  = aws_s3_bucket.pfr.id
      S3_PREFIX  = "pfr"
      DEBUG      = "1" # Set to "1" to enable debug logging
    }
  }
}

###########################
# Lambda IAM role & policy
###########################

# Trust policy
data "aws_iam_policy_document" "lambda_assume" {
  statement {
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["lambda.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "pfr_weekly" {
  name               = "pfr-weekly-role-${var.season}"
  assume_role_policy = data.aws_iam_policy_document.lambda_assume.json
}

# Inline permissions: logs + DDB write + optional S3 put
data "aws_iam_policy_document" "pfr_inline" {
  statement {
    sid = "Logs"
    actions = [
      "logs:CreateLogGroup",
      "logs:CreateLogStream",
      "logs:PutLogEvents"
    ]
    resources = ["*"]
  }

  statement {
    sid = "DDBWrite"
    actions = [
      "dynamodb:BatchWriteItem",
      "dynamodb:PutItem"
    ]
    resources = [
      aws_dynamodb_table.players.arn
    ]
  }

  # Remove this block if you are not archiving CSVs to S3
  statement {
    sid = "S3Put"
    actions = [
      "s3:PutObject"
    ]
    resources = [
      "${aws_s3_bucket.pfr.arn}/*"
    ]
  }
}

resource "aws_iam_role_policy" "pfr_inline" {
  name   = "pfr-weekly-inline"
  role   = aws_iam_role.pfr_weekly.id
  policy = data.aws_iam_policy_document.pfr_inline.json
}