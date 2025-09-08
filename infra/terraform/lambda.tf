##########################
# Lambda (Go)
##########################

# Build your binary as "main" and zip it:
# GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o main ./cmd/pfr-weekly
# zip -9 pfr-weekly.zip main

resource "aws_lambda_function" "pfr_weekly" {
  function_name    = "pfr-weekly-${var.season}"
  filename         = "${local.artifacts_dir}/pfr-weekly.zip"
  source_code_hash = filebase64sha256("${path.module}/artifacts/pfr-weekly.zip")
  handler          = "bootstrap"
  runtime          = "provided.al2023"
  architectures    = ["x86_64"]
  timeout          = 900
  memory_size      = 1024
  role             = aws_iam_role.pfr_weekly.arn

  environment {
    variables = {
      TABLE_NAME = aws_dynamodb_table.defensive_players_by_team.name
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
      aws_dynamodb_table.defensive_players_by_team.arn
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

# Package path example: artifacts/pfr-snaps.zip
resource "aws_lambda_function" "pfr_snaps_2024" {
  function_name = "pfr-snaps-2024"
  role          = aws_iam_role.pfr_snaps_role.arn
  handler       = "bootstrap"
  runtime       = "provided.al2023"
  filename      = "${local.artifacts_dir}/pfr-snaps.zip"
  memory_size   = 1024
  timeout       = 600
  architectures = ["x86_64"]
  environment {
    variables = {
      MODE                   = "ingest_snaps_by_game"
      SNAP_TABLE_NAME        = aws_dynamodb_table.defensive_snaps_by_game.name
      TABLE_NAME             = aws_dynamodb_table.defensive_players_by_team.name
      SEASON                 = "2024"
      TEAM_DELAY_MS          = "500"
      HTTP_MAX_ATTEMPTS      = "7"
      HTTP_RETRY_BASE_MS     = "400"
      HTTP_RETRY_MAX_MS      = "6000"
      HTTP_COOLDOWN_MS       = "9000"
      HTTP_FINAL_COOLDOWN_MS = "15000"
      PASS_MAX               = "3"
      SHUFFLE_TEAMS          = "1"
      DEBUG                  = "1"
    }
  }
}

resource "aws_iam_role" "pfr_snaps_role" {
  name               = "pfr-snaps-role-2024"
  assume_role_policy = data.aws_iam_policy_document.lambda_assume.json
}

data "aws_iam_policy_document" "pfr_snaps_ddb" {
  statement {
    actions = [
      "dynamodb:BatchWriteItem",
      "dynamodb:PutItem",
      "dynamodb:UpdateItem",
      "dynamodb:Query",
      "dynamodb:DescribeTable"
    ]
    resources = [
      aws_dynamodb_table.defensive_snaps_by_game.arn,
      aws_dynamodb_table.defensive_players_by_team.arn,
      "${aws_dynamodb_table.defensive_players_by_team.arn}/index/*"
    ]
  }
  # CloudWatch logs
  statement {
    actions   = ["logs:CreateLogGroup", "logs:CreateLogStream", "logs:PutLogEvents"]
    resources = ["*"]
  }
}

resource "aws_iam_policy" "pfr_snaps_ddb" {
  name   = "pfr-snaps-ddb-2024"
  policy = data.aws_iam_policy_document.pfr_snaps_ddb.json
}

resource "aws_iam_role_policy_attachment" "pfr_snaps_ddb_attach" {
  role       = aws_iam_role.pfr_snaps_role.name
  policy_arn = aws_iam_policy.pfr_snaps_ddb.arn
}