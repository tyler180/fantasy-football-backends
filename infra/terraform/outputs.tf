output "glue_db" {
  value = aws_glue_catalog_database.curated.name
}

output "athena_workgroup" {
  value = aws_athena_workgroup.wg.name
}

output "players_table" {
  value = aws_glue_catalog_table.players.name
}

output "rosters_weekly_table" {
  value = aws_glue_catalog_table.rosters_weekly.name
}

output "snap_counts_table" {
  value = aws_glue_catalog_table.snap_counts.name
}

output "ddb_defensive_starters_allgames" {
  value = aws_dynamodb_table.defensive_starters_allgames.name
}