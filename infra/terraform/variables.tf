variable "username" {
  sensitive = true
  type      = string
}

variable "password" {
  sensitive = true
  type      = string
}

variable "api_key" {
  sensitive = true
  type      = string
}

variable "league_id" {
  type = string
}

variable "franchise_id" {
  type = string
}

variable "league_year" {
  type    = string
  default = "2025"
}

variable "setjson" {
  type    = string
  default = "1"
}

variable "setxml" {
  type    = string
  default = "0"
}

variable "season" {
  type    = string
  default = "2025"
}

variable "aws_region" {
  type    = string
  default = "us-west-2"
}

# variable "curated_bucket" {
#   type        = string
#   description = "S3 bucket for curated Parquet datasets"
#   default     = "nflverse-curated-datasets"
# }

# variable "athena_output_bucket" {
#   type        = string
#   description = "S3 bucket for Athena query results"
#   default     = "nflverse-athena-query-results"
# }

# variable "glue_database" {
#   type    = string
#   default = "nflverse_curated"
# }

# variable "athena_workgroup" {
#   type    = string
#   default = "fantasy_football_backends"
# }

# variable "curated_prefix" {
#   type    = string
#   default = "nflverse_curated"
# }

# Lambda zips (built by your Makefile)
# variable "zip_nflverse_curator" {
#   type    = string
#   default = "${path.module}/../artifacts/nflverse-curator.zip"
# }

# variable "zip_athena_materializer" {
#   type    = string
#   default = "${path.module}/../artifacts/athena-materializer.zip"
# }

# App defaults
variable "season_default" {
  type    = string
  default = "2024"
}
variable "max_age_default" {
  type    = string
  default = "24"
}
variable "starter_pct_default" {
  type    = string
  default = "50"
}