locals {
  region = "us-west-2"

  tags = {
    Environment = "test"
    Project     = "ff-backends"
  }

  # Common SerDe / formats for Parquet tables
  parquet_serde  = "org.apache.hadoop.hive.ql.io.parquet.serde.ParquetHiveSerDe"
  parquet_input  = "org.apache.hadoop.hive.ql.io.parquet.MapredParquetInputFormat"
  parquet_output = "org.apache.hadoop.hive.ql.io.parquet.MapredParquetOutputFormat"

  curated_root_players        = "s3://${aws_s3_bucket.curated.bucket}/${local.curated.prefix}/players/"
  curated_root_rosters_weekly = "s3://${aws_s3_bucket.curated.bucket}/${local.curated.prefix}/rosters_weekly/"
  curated_root_snap_counts    = "s3://${aws_s3_bucket.curated.bucket}/${local.curated.prefix}/snap_counts/"

  # NFL team codes for partition projection (snap_counts)
  team_codes = "ARI,ATL,BAL,BUF,CAR,CHI,CIN,CLE,DAL,DEN,DET,GB,HOU,IND,JAX,KC,LV,LAC,LA,MIA,MIN,NE,NO,NYG,NYJ,PHI,PIT,SF,SEA,TB,TEN,WAS"

  artifacts_dir           = "${path.module}/../../artifacts"
  zip_athena_materializer = "${path.module}/../artifacts/athena-materializer.zip"
  zip_nflverse_curator    = "${path.module}/../artifacts/nflverse-curator.zip"
  curated = {
    db_name               = "nflverse_curated"
    bucket_name           = "nflverse-curated-datasets"
    athena_output_bucket  = "nflverse-athena-query-results"
    prefix                = "nflverse_curated"
    athena_workgroup_name = "fantasy_football_backends"
  }
  athena_workgroup_name = "fantasy_football_backends"
  crawler_name          = "nflverse-curated-crawler"
}