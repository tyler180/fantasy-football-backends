# # API Gateway (Optional: If you want an HTTP trigger for Lambda)
# resource "aws_apigatewayv2_api" "scraper_api" {
#   name          = "ScraperAPI"
#   protocol_type = "HTTP"
# }

# resource "aws_apigatewayv2_integration" "lambda_integration" {
#   api_id                 = aws_apigatewayv2_api.scraper_api.id
#   integration_type       = "AWS_PROXY"
#   integration_uri        = aws_lambda_function.player_scraper.arn
#   payload_format_version = "2.0"
# }

# resource "aws_apigatewayv2_route" "scraper_route" {
#   api_id    = aws_apigatewayv2_api.scraper_api.id
#   route_key = "GET /scrape"
#   target    = "integrations/${aws_apigatewayv2_integration.lambda_integration.id}"
# }

# resource "aws_apigatewayv2_stage" "scraper_stage" {
#   api_id      = aws_apigatewayv2_api.scraper_api.id
#   name        = "$default"
#   auto_deploy = true
# }