resource "aws_iam_role" "crawler" {
  name               = "nflverse-curated-crawler-role"
  assume_role_policy = data.aws_iam_policy_document.crawler_assume.json
}

data "aws_iam_policy_document" "crawler_assume" {
  statement {
    effect = "Allow"
    principals {
      type        = "Service"
      identifiers = ["glue.amazonaws.com"]
    }
    actions = ["sts:AssumeRole"]
  }
}

# Managed Glue service role + S3 read to curated bucket
resource "aws_iam_role_policy_attachment" "crawler_service" {
  role       = aws_iam_role.crawler.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSGlueServiceRole"
}

data "aws_iam_policy_document" "crawler_s3_read" {
  statement {
    effect = "Allow"
    actions = [
      "s3:GetObject", "s3:ListBucket", "s3:GetBucketLocation"
    ]
    resources = [
      aws_s3_bucket.curated.arn,
      "${aws_s3_bucket.curated.arn}/${local.curated.prefix}*"
    ]
  }
}
resource "aws_iam_policy" "crawler_s3_read" {
  name   = "${local.crawler_name}-s3-read"
  policy = data.aws_iam_policy_document.crawler_s3_read.json
}
resource "aws_iam_role_policy_attachment" "crawler_s3_read_attach" {
  role       = aws_iam_role.crawler.name
  policy_arn = aws_iam_policy.crawler_s3_read.arn
}

# resource "aws_glue_crawler" "curated" {
#   name          = local.crawler_name
#   role          = aws_iam_role.crawler.arn
#   database_name = aws_glue_catalog_database.curated.name

#   s3_target {
#     path = "${aws_s3_bucket.curated.arn}/${local.curated.prefix}"
#   }

#   configuration = jsonencode({
#     Version  = 1.0,
#     Grouping = { TableGroupingPolicy = "CombineCompatibleSchemas" }
#     CrawlerOutput = {
#       Partitions = { AddOrUpdateBehavior = "InheritFromTable" }
#     }
#     SchemaChangePolicy = {
#       UpdateBehavior = "UPDATE_IN_DATABASE"
#       DeleteBehavior = "LOG"
#     }
#   })
# }

resource "aws_glue_crawler" "curated" {
  name          = local.crawler_name
  role          = aws_iam_role.crawler.arn
  database_name = aws_glue_catalog_database.curated.name

  s3_target {
    path = "s3://${aws_s3_bucket.curated.id}/${local.curated.prefix}"
  }

  # Only keys supported by the JSON "configuration" go here.
  configuration = jsonencode({
    Version = 1.0
    Grouping = {
      TableGroupingPolicy = "CombineCompatibleSchemas"
    }
    CrawlerOutput = {
      Partitions = { AddOrUpdateBehavior = "InheritFromTable" }
      Tables     = { AddOrUpdateBehavior = "MergeNewColumns" }
    }
  })

  # <-- This is the correct place for schema change policy
  schema_change_policy {
    update_behavior = "UPDATE_IN_DATABASE"
    delete_behavior = "LOG" # or "DEPRECATE_IN_DATABASE"
  }

  # Optional but nice to be explicit
  recrawl_policy {
    recrawl_behavior = "CRAWL_EVERYTHING" # or "CRAWL_NEW_FOLDERS_ONLY"
  }

  # Optional
  lineage_configuration {
    crawler_lineage_settings = "DISABLE"
  }
}