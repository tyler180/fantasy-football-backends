# # EventBridge Rule for Nightly Trigger
# resource "aws_cloudwatch_event_rule" "nightly_trigger" {
#   name                = "nightly_lambda_backend_triggers"
#   description         = "Trigger the backend lambda functions nightly"
#   schedule_expression = "cron(0 0 * * ? *)" # Midnight UTC daily
# }

# # EventBridge Target for Lambda
# resource "aws_cloudwatch_event_target" "player_scraper_lambda_target" {
#   rule = aws_cloudwatch_event_rule.nightly_trigger.name
#   arn  = aws_lambda_function.player_scraper.arn
# }

# resource "aws_cloudwatch_event_target" "free_agent_scraper_lambda_target" {
#   rule = aws_cloudwatch_event_rule.nightly_trigger.name
#   arn  = aws_lambda_function.free_agent_scraper.arn
# }
# EventBridge schedule -> Fetcher
resource "aws_cloudwatch_event_rule" "weekly" {
  name                = "pfr-weekly-${var.season}"
  schedule_expression = "cron(10 15 ? * TUE *)" # Tuesdays 15:10 UTC (~09:10 America/Denver during MDT)
}

resource "aws_cloudwatch_event_target" "weekly_target" {
  rule      = aws_cloudwatch_event_rule.weekly.name
  target_id = "pfr-weekly"
  arn       = aws_lambda_function.pfr_weekly.arn
}

resource "aws_lambda_permission" "allow_events" {
  statement_id  = "AllowEventsInvoke"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.pfr_weekly.function_name
  principal     = "events.amazonaws.com"
  source_arn    = aws_cloudwatch_event_rule.weekly.arn
}