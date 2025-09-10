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

resource "aws_cloudwatch_event_rule" "snaps_chunk0" {
  name                = "snaps-chunk0-weekly"
  schedule_expression = "cron(0 15 ? * TUE *)" # Tuesdays 15:00 UTC (morning MT)
}
resource "aws_cloudwatch_event_target" "snaps_chunk0_target" {
  rule      = aws_cloudwatch_event_rule.snaps_chunk0.name
  target_id = "snaps0"
  arn       = aws_lambda_function.pfr_snaps_2024.arn
  input     = jsonencode({ mode = "ingest_snaps_by_game", season = "2024", team_chunk_total = 2, team_chunk_index = 0 })
}

resource "aws_cloudwatch_event_rule" "snaps_chunk1" {
  name                = "snaps-chunk1-weekly"
  schedule_expression = "cron(10 15 ? * TUE *)"
}
resource "aws_cloudwatch_event_target" "snaps_chunk1_target" {
  rule      = aws_cloudwatch_event_rule.snaps_chunk1.name
  target_id = "snaps1"
  arn       = aws_lambda_function.pfr_snaps_2024.arn
  input     = jsonencode({ mode = "ingest_snaps_by_game", season = "2024", team_chunk_total = 2, team_chunk_index = 1 })
}

resource "aws_cloudwatch_event_rule" "snaps_trends" {
  name                = "snaps-trends-weekly"
  schedule_expression = "cron(25 15 ? * TUE *)"
}
resource "aws_cloudwatch_event_target" "snaps_trends_target" {
  rule      = aws_cloudwatch_event_rule.snaps_trends.name
  target_id = "snapstrends"
  arn       = aws_lambda_function.pfr_snaps_2024.arn
  input     = jsonencode({ mode = "materialize_snap_trends", season = "2024" })
}

# Weekly ingestion: Monday 03:00 UTC
resource "aws_cloudwatch_event_rule" "curator_weekly" {
  name                = "nflverse-curator-weekly"
  schedule_expression = "cron(0 3 ? * MON *)"
}

resource "aws_cloudwatch_event_target" "curator_target" {
  rule      = aws_cloudwatch_event_rule.curator_weekly.name
  target_id = "nflverse-curator"
  arn       = aws_lambda_function.nflverse_curator.arn
  input     = jsonencode({ datasets = ["players", "rosters_weekly", "snap_counts"] })
}

resource "aws_lambda_permission" "curator_invoke" {
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.nflverse_curator.function_name
  principal     = "events.amazonaws.com"
  source_arn    = aws_cloudwatch_event_rule.curator_weekly.arn
}

# Weekly materialization: Monday 03:05 UTC
resource "aws_cloudwatch_event_rule" "materializer_weekly" {
  name                = "athena-materializer-weekly"
  schedule_expression = "cron(5 3 ? * MON *)"
}

resource "aws_cloudwatch_event_target" "materializer_target" {
  rule      = aws_cloudwatch_event_rule.materializer_weekly.name
  target_id = "athena-materializer"
  arn       = aws_lambda_function.athena_materializer.arn
  input     = jsonencode({ season = tonumber(var.season_default) })
}

resource "aws_lambda_permission" "materializer_invoke" {
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.athena_materializer.function_name
  principal     = "events.amazonaws.com"
  source_arn    = aws_cloudwatch_event_rule.materializer_weekly.arn
}