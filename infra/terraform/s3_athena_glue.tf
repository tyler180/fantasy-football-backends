resource "aws_glue_catalog_database" "curated" {
  name = local.curated.db_name
}

# --- S3 buckets ---
resource "aws_s3_bucket" "curated" {
  bucket = local.curated.bucket_name
}

resource "aws_s3_bucket_server_side_encryption_configuration" "curated_sse" {
  bucket = aws_s3_bucket.curated.id
  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

resource "aws_s3_bucket" "athena_out" {
  bucket = local.curated.athena_output_bucket
}

resource "aws_s3_bucket_server_side_encryption_configuration" "athena_out_sse" {
  bucket = aws_s3_bucket.athena_out.id
  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

# --- Glue database ---
# resource "aws_glue_catalog_database" "db" {
#   name = var.glue_database
# }

# --- Athena workgroup ---
resource "aws_athena_workgroup" "wg" {
  name = local.athena_workgroup_name
  configuration {
    result_configuration {
      output_location = "s3://${aws_s3_bucket.athena_out.bucket}/results/"
    }
    enforce_workgroup_configuration = false
  }
}

# --- Glue tables with partition projection ---

# 1) players (no partitions)
resource "aws_glue_catalog_table" "players" {
  name          = "players"
  database_name = aws_glue_catalog_database.curated.name
  table_type    = "EXTERNAL_TABLE"

  storage_descriptor {
    location      = local.curated_root_players
    input_format  = local.parquet_input
    output_format = local.parquet_output

    ser_de_info {
      name                  = "ParquetHiveSerDe"
      serialization_library = local.parquet_serde
    }

    columns {
      name = "pfr_id"
      type = "string"
    }
    columns {
      name = "gsis_id"
      type = "string"
    }
    columns {
      name = "full_name"
      type = "string"
    }
    columns {
      name = "birth_date"
      type = "string" # keep as string; cast in SQL if needed
    }
    columns {
      name = "position"
      type = "string"
    }
  }

  parameters = {
    EXTERNAL              = "TRUE"
    "parquet.compression" = "SNAPPY"
    "classification"      = "parquet"
    "projection.enabled"  = "false"
  }
}

# 2) rosters_weekly partitioned by season, week
resource "aws_glue_catalog_table" "rosters_weekly" {
  name          = "rosters_weekly"
  database_name = aws_glue_catalog_database.curated.name
  table_type    = "EXTERNAL_TABLE"

  storage_descriptor {
    location      = local.curated_root_rosters_weekly
    input_format  = local.parquet_input
    output_format = local.parquet_output

    ser_de_info {
      name                  = "ParquetHiveSerDe"
      serialization_library = local.parquet_serde
    }

    # Non-partition columns
    columns {
      name = "team"
      type = "string"
    }
    columns {
      name = "player_id"
      type = "string"
    }
    columns {
      name = "pfr_id"
      type = "string"
    }
    columns {
      name = "full_name"
      type = "string"
    }
    columns {
      name = "position"
      type = "string"
    }
    columns {
      name = "status"
      type = "string"
    }
  }

  partition_keys {
    name = "season"
    type = "string"
  }
  partition_keys {
    name = "week"
    type = "string"
  }

  parameters = {
    EXTERNAL              = "TRUE"
    "parquet.compression" = "SNAPPY"
    "classification"      = "parquet"
    "projection.enabled"  = "true"

    # season projection
    "projection.season.type"  = "integer"
    "projection.season.range" = "2000,2035"

    # week projection (00 to 22; we write '01','02',... so format matters)
    "projection.week.type"   = "integer"
    "projection.week.range"  = "1,22"
    "projection.week.format" = "%02d"

    # template
    "storage.location.template" = "${local.curated_root_rosters_weekly}season=$${season}/week=$${week}/"
  }
}

# 3) snap_counts partitioned by season, team
resource "aws_glue_catalog_table" "snap_counts" {
  name          = "snap_counts"
  database_name = aws_glue_catalog_database.curated.name
  table_type    = "EXTERNAL_TABLE"

  storage_descriptor {
    location      = local.curated_root_snap_counts
    input_format  = local.parquet_input
    output_format = local.parquet_output

    ser_de_info {
      name                  = "ParquetHiveSerDe"
      serialization_library = local.parquet_serde
    }

    # Non-partition columns
    columns {
      name = "week"
      type = "string"
    }
    columns {
      name = "player"
      type = "string"
    }
    columns {
      name = "player_id"
      type = "string"
    }
    columns {
      name = "offense_pct"
      type = "double"
    }
    columns {
      name = "defense_pct"
      type = "double"
    }
    columns {
      name = "st_pct"
      type = "double"
    }
  }

  partition_keys {
    name = "season"
    type = "string"
  }
  partition_keys {
    name = "team"
    type = "string"
  }

  parameters = {
    EXTERNAL              = "TRUE"
    "parquet.compression" = "SNAPPY"
    "classification"      = "parquet"
    "projection.enabled"  = "true"

    # season projection
    "projection.season.type"  = "integer"
    "projection.season.range" = "2000,2035"

    # team projection
    "projection.team.type"   = "enum"
    "projection.team.values" = local.team_codes

    # template
    "storage.location.template" = "${local.curated_root_snap_counts}season=$${season}/team=$${team}/"
  }
}