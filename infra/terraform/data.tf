# data "aws_organizations_organization" "this" {}

data "aws_caller_identity" "current" {}

data "aws_kms_alias" "lambda" {
  name = "alias/aws/lambda"
}

# data "archive_file" "zip" {
#   type        = "zip"
#   source_dir  = "${path.module}/ffnotifier"
#   output_path = "${path.module}/ffnotifier.zip"
# }

# data "terraform_remote_state" "secrets_layer" {
#   backend = "s3"
#   config = {
#     bucket = "ff-test-retrievesecret-state"
#     key    = "terraform.tfstate"
#     region = local.region
#   }
# }

data "aws_iam_policy_document" "lambda_kms_use" {
  statement {
    actions = [
      "kms:Decrypt",
      "kms:Encrypt",
      "kms:GenerateDataKey",
      "kms:GenerateDataKeyWithoutPlaintext",
      "kms:DescribeKey"
    ]
    resources = ["arn:aws:kms:us-west-2:${data.aws_caller_identity.current.account_id}:key/3131e78c-f9bf-4aed-9553-de0d42b503db"]
  }
}
resource "aws_iam_policy" "lambda_kms_use" {
  name   = "athena-materializer-kms-use"
  policy = data.aws_iam_policy_document.lambda_kms_use.json
}
resource "aws_iam_role_policy_attachment" "lambda_kms_use_attach" {
  role       = aws_iam_role.athena_materializer.name
  policy_arn = aws_iam_policy.lambda_kms_use.arn
}