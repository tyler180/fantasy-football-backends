locals {
  region = "us-west-2"

  tags = {
    Environment = "test"
    Project     = "ff-backends"
  }

  artifacts_dir = "${path.module}/../../artifacts"
}