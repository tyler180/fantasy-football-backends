terraform {
  required_version = ">= 1.1.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 4.9.0"
    }
    local = {
      source  = "hashicorp/local"
      version = ">= 1.3"
    }
  }
}

provider "aws" {
  region = "us-west-2" # Update to your preferred region
}