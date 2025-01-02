# IAM Role for Lambda
resource "aws_iam_role" "lambda_exec_role" {
  name = "lambda_exec_role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "lambda.amazonaws.com"
        }
      }
    ]
  })
}

# Attach Policies to the IAM Role
resource "aws_iam_role_policy_attachment" "lambda_policy_attach" {
  role       = aws_iam_role.lambda_exec_role.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
}

resource "aws_iam_role_policy_attachment" "dynamodb_access" {
  role       = aws_iam_role.lambda_exec_role.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonDynamoDBFullAccess"
}

data "aws_iam_policy_document" "exec_policy" {

  statement {
    sid = "SMReadAccess"
    actions = [
      "secretsmanager:GetSecretValue",
      "secretsmanager:Get*"
    ]
    resources = ["*"]
  }
}

resource "aws_iam_policy" "policy" {
  name   = "sm-get-permissions"
  policy = data.aws_iam_policy_document.exec_policy.json
}

resource "aws_iam_role_policy_attachment" "sm_access" {
  role       = aws_iam_role.lambda_exec_role.name
  policy_arn = aws_iam_policy.policy.arn
}