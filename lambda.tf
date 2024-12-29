# player stats Lambda Function
resource "aws_lambda_function" "player_scraper" {
  function_name = "player_scraper_lambda"
  role          = aws_iam_role.lambda_exec_role.arn
  handler       = "bootstrap"
  runtime       = "provided.al2023"

  filename         = "${path.module}/lambda.zip"
  source_code_hash = filebase64sha256("${path.module}/lambda.zip")

  layers = [
    data.terraform_remote_state.secrets_layer.outputs.secrets_layer_arn
  ]

  environment {
    variables = {
      DYNAMODB_TABLE = aws_dynamodb_table.players_table.name
      SECRET_NAME    = module.secrets-manager.secret_name
    }
  }

  tags = {
    Environment = "dev"
    Project     = "PlayerScraper"
  }
}

# Permission to Allow EventBridge to Invoke player_scraper
resource "aws_lambda_permission" "allow_eventbridge_invoke_player_scraper" {
  statement_id  = "AllowEventBridgeInvoke"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.player_scraper.function_name
  principal     = "events.amazonaws.com"
  source_arn    = aws_cloudwatch_event_rule.nightly_trigger.arn
}

# MFL free agent Lambda Function
resource "aws_lambda_function" "free_agent_scraper" {
  function_name = "mfl-free-agents"
  role          = aws_iam_role.lambda_exec_role.arn
  handler       = "index.lambdaHandler"
  runtime       = "provided.al2023"
  architectures = ["arm64"]

  filename         = "${path.module}/lambda.zip"
  source_code_hash = filebase64sha256("${path.module}/lambda.zip")

  layers = [
    data.terraform_remote_state.secrets_layer.outputs.secrets_layer_arn
  ]

  environment {
    variables = {
      DYNAMODB_TABLE = aws_dynamodb_table.players_table.name
      SECRET_NAME    = module.secrets-manager.secret_name
    }
  }

  tags = {
    Environment = "dev"
    Project     = "PlayerScraper"
  }
}

# Permission to Allow EventBridge to Invoke mfl-free-agents
resource "aws_lambda_permission" "allow_eventbridge_invoke_free_agent_scraper" {
  statement_id  = "AllowEventBridgeInvoke"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.free_agent_scraper.function_name
  principal     = "events.amazonaws.com"
  source_arn    = aws_cloudwatch_event_rule.nightly_trigger.arn
}