# EventBridge Rule for Nightly Trigger
resource "aws_cloudwatch_event_rule" "nightly_trigger" {
  name                = "nightly_lambda_backend_triggers"
  description         = "Trigger the backend lambda functions nightly"
  schedule_expression = "cron(0 0 * * ? *)" # Midnight UTC daily
}

# EventBridge Target for Lambda
resource "aws_cloudwatch_event_target" "player_scraper_lambda_target" {
  rule = aws_cloudwatch_event_rule.nightly_trigger.name
  arn  = aws_lambda_function.player_scraper.arn
}

resource "aws_cloudwatch_event_target" "free_agent_scraper_lambda_target" {
  rule = aws_cloudwatch_event_rule.nightly_trigger.name
  arn  = aws_lambda_function.free_agent_scraper.arn
}
