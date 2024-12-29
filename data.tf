# data "aws_organizations_organization" "this" {}

data "aws_caller_identity" "current" {}

# data "archive_file" "zip" {
#   type        = "zip"
#   source_dir  = "${path.module}/ffnotifier"
#   output_path = "${path.module}/ffnotifier.zip"
# }

data "terraform_remote_state" "secrets_layer" {
  backend = "s3"
  config = {
    bucket = "ff-test-retrievesecret-state"
    key    = "terraform.tfstate"
    region = local.region
  }
}