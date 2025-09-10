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

# --- nflverse-curator (Go / custom runtime) ---
resource "aws_iam_role" "nflverse_curator_role" {
  name = "nflverse-curator-role"
  assume_role_policy = jsonencode({
    Version   = "2012-10-17",
    Statement = [{ Effect = "Allow", Principal = { Service = "lambda.amazonaws.com" }, Action = "sts:AssumeRole" }]
  })
}

resource "aws_iam_role_policy" "nflverse_curator_policy" {
  name   = "nflverse-curator-policy"
  role   = aws_iam_role.nflverse_curator_role.id
  policy = data.aws_iam_policy_document.s3_and_logs.json
}

data "aws_iam_policy_document" "s3_and_logs" {
  version = "2012-10-17"

  statement {
    effect = "Allow"
    actions = [
      "s3:PutObject",
      "s3:GetObject",
      "s3:ListBucket",
      "s3:DeleteObject",
      "s3:GetBucketLocation",
    ]
    resources = [
      aws_s3_bucket.curated.arn,
      "${aws_s3_bucket.curated.arn}/*",
    ]
  }

  statement {
    effect = "Allow"
    actions = [
      "logs:CreateLogGroup",
      "logs:CreateLogStream",
      "logs:PutLogEvents",
    ]
    resources = ["*"]
  }
}

resource "aws_lambda_function" "nflverse_curator" {
  function_name = "nflverse-curator"
  role          = aws_iam_role.nflverse_curator_role.arn
  runtime       = "provided.al2023"
  handler       = "bootstrap"
  filename      = local.zip_nflverse_curator
  timeout       = 900
  memory_size   = 512
  kms_key_arn   = null

  environment {
    variables = {
      CURATED_BUCKET   = aws_s3_bucket.curated.bucket
      CURATED_PREFIX   = local.curated.prefix
      GLUE_DATABASE    = aws_glue_catalog_database.curated.name
      ATHENA_WORKGROUP = aws_athena_workgroup.wg.name
      ATHENA_OUTPUT    = "s3://${aws_s3_bucket.athena_out.bucket}/results/"
      SEASON           = var.season_default
      MAX_AGE          = var.max_age_default
    }
  }
}

# --- athena-materializer (Go / custom runtime) ---
# resource "aws_iam_role" "athena_materializer_role" {
#   name               = "athena-materializer-role"
#   assume_role_policy = data.aws_iam_policy_document.lambda_assume.json
# }

resource "aws_iam_role_policy" "athena_materializer_policy" {
  name   = "athena-materializer-policy"
  role   = aws_iam_role.athena_materializer.id
  policy = data.aws_iam_policy_document.athena_s3_dynamodb_logs.json
}

data "aws_iam_policy_document" "athena_s3_dynamodb_logs" {
  version = "2012-10-17"

  statement {
    effect = "Allow"
    actions = [
      "athena:StartQueryExecution",
      "athena:GetQueryExecution",
      "athena:GetQueryResults",
      "athena:StopQueryExecution",
      "athena:GetWorkGroup",
    ]
    resources = ["*"]
  }

  statement {
    effect = "Allow"
    actions = [
      "s3:GetObject",
      "s3:PutObject",
      "s3:ListBucket",
      "s3:DeleteObject",
      "s3:GetBucketLocation",
    ]
    resources = [
      aws_s3_bucket.curated.arn,
      "${aws_s3_bucket.curated.arn}/*",
      aws_s3_bucket.athena_out.arn,
      "${aws_s3_bucket.athena_out.arn}/*",
    ]
  }

  statement {
    effect = "Allow"
    actions = [
      "dynamodb:BatchWriteItem",
      "dynamodb:PutItem",
      "dynamodb:UpdateItem",
      "dynamodb:DescribeTable",
    ]
    resources = [
      aws_dynamodb_table.defensive_starters_allgames.arn
    ]
  }

  statement {
    effect = "Allow"
    actions = [
      "logs:CreateLogGroup",
      "logs:CreateLogStream",
      "logs:PutLogEvents",
    ]
    resources = ["*"]
  }
}

# data "aws_iam_policy_document" "athena_mat_athena" {
#   statement {
#     sid     = "AthenaQueries"
#     actions = [
#       "athena:StartQueryExecution",
#       "athena:GetQueryExecution",
#       "athena:GetQueryResults",
#       "athena:StopQueryExecution",
#       "athena:ListQueryExecutions",
#       "athena:GetWorkGroup",
#       "athena:ListWorkGroups"
#     ]
#     resources = [
#       "arn:aws:athena:${var.aws_region}:${var.aws_account_id}:workgroup/${var.athena_workgroup}"
#     ]
#   }
# }

data "aws_iam_policy_document" "athena_materializer_glue" {
  statement {
    sid = "GlueCatalogRead"
    actions = [
      "glue:GetCatalogImportStatus",
      "glue:GetDatabase",
      "glue:GetDatabases",
      "glue:GetTable",
      "glue:GetTables",
      "glue:GetPartition",
      "glue:GetPartitions"
    ]
    resources = ["*"]
  }

  statement {
    sid = "GlueDbTableCrud"
    actions = [
      "glue:CreateTable",
      "glue:UpdateTable",
      "glue:DeleteTable",
      "glue:BatchCreatePartition",
      "glue:BatchDeletePartition"
    ]
    resources = ["*"]
    # resources = [
    #   "arn:aws:glue:${var.aws_region}:${data.aws_caller_identity.current.account_id}:database/${local.curated.db_name}",
    #   "arn:aws:glue:${var.aws_region}:${data.aws_caller_identity.current.account_id}:table/${local.curated.db_name}/*",
    #   aws_glue_catalog_table.players.arn,
    #   aws_glue_catalog_table.rosters_weekly.arn,
    #   aws_glue_catalog_table.snap_counts.arn
    # ]
  }

  statement {
    sid = "AthenaQueries"
    actions = [
      "athena:StartQueryExecution",
      "athena:GetQueryExecution",
      "athena:GetQueryResults",
      "athena:StopQueryExecution",
      "athena:ListQueryExecutions",
      "athena:GetWorkGroup",
      "athena:ListWorkGroups"
    ]
    resources = [
      "arn:aws:athena:${var.aws_region}:${data.aws_caller_identity.current.account_id}:workgroup/${aws_athena_workgroup.wg.name}"
    ]
  }
}

resource "aws_iam_policy" "athena_materializer_glue" {
  name        = "${aws_iam_role.athena_materializer.name}-glue"
  description = "Glue permissions for Athena CTAS/DROP on ${local.curated.db_name}"
  policy      = data.aws_iam_policy_document.athena_materializer_glue.json
}

resource "aws_iam_role_policy_attachment" "athena_materializer_glue_attach" {
  role       = aws_iam_role.athena_materializer.name
  policy_arn = aws_iam_policy.athena_materializer_glue.arn
}

resource "aws_lambda_function" "athena_materializer" {
  function_name = "athena-materializer"
  role          = aws_iam_role.athena_materializer.arn
  runtime       = "provided.al2023"
  handler       = "bootstrap"
  filename      = local.zip_athena_materializer
  timeout       = 900
  memory_size   = 512
  kms_key_arn   = null

  environment {
    variables = {
      ATHENA_DB        = aws_glue_catalog_database.curated.name
      CURATED_BUCKET   = aws_s3_bucket.curated.bucket
      CURATED_PREFIX   = local.curated.prefix
      ATHENA_WORKGROUP = aws_athena_workgroup.wg.name
      ATHENA_OUTPUT    = "s3://${aws_s3_bucket.athena_out.bucket}/results/"
      SERVE_TABLE      = aws_dynamodb_table.defensive_starters_allgames.name
      SEASON           = var.season_default
      MAX_AGE          = var.max_age_default
      STARTER_PCT      = var.starter_pct_default
    }
  }
}

# data "aws_iam_policy_document" "lambda_assume" {
#   statement {
#     effect  = "Allow"
#     actions = ["sts:AssumeRole"]
#     principals {
#       type        = "Service"
#       identifiers = ["lambda.amazonaws.com"]
#     }
#   }
# }

resource "aws_iam_role" "athena_materializer" {
  name               = "athena-materializer"
  assume_role_policy = data.aws_iam_policy_document.lambda_assume.json
}