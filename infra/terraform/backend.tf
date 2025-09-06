terraform {
  required_version = ">= 1.0.0"

  backend "s3" {
    region         = "us-west-2"
    bucket         = "rmw-terraform-statefiles"
    key            = "fantasy-football-backends.tfstate"
    encrypt        = "true"
    dynamodb_table = "terraform-lock-table"
  }
}