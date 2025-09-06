# DynamoDB table (PK: Season, SK: Player)
resource "aws_dynamodb_table" "players" {
  name         = "defensive_players_${var.season}"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "Season"
  range_key    = "Player"
  attribute {
    name = "Season"
    type = "S"
  }
  attribute {
    name = "Player"
    type = "S"
  }
}

resource "aws_dynamodb_table" "mfl_free_agents" {
  name         = "mfl_free_agents"
  billing_mode = "PAY_PER_REQUEST" # Use on-demand mode for simplicity
  hash_key     = "playerID"        # Primary key: playerID

  attribute {
    name = "playerID"
    type = "S" # String type
  }

  attribute {
    name = "position"
    type = "S" # String type for GSI
  }

  # Global Secondary Index for querying by position
  global_secondary_index {
    name            = "PositionIndex"
    hash_key        = "position"
    projection_type = "ALL" # Include all attributes in the index
  }

  # Tags for resource identification
  tags = {
    Environment = "Dev"
    Project     = "FantasyFootball"
  }
}