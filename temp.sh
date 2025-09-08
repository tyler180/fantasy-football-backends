aws lambda update-function-configuration \
  --function-name pfr-weekly-2024 \
  --environment "Variables={TABLE_NAME=defensive_players_2024,SEASON=2024,MAX_AGE=24,POSITIONS=DE,DT,NT,DL,EDGE,LB,ILB,OLB,MLB,CB,DB,S,FS,SS,SAF,NB,DEBUG=1,ASSUME_GS_EQUALS_G=1}"


  aws lambda update-function-configuration \
  --region us-west-2 \
  --function-name pfr-weekly-2024 \
  --environment '{
    "Variables": {
      "ROSTER_TABLE_NAME": "nfl_roster_rows",
      "TABLE_NAME": "defensive_players_2024",
      "SEASON": "2024",
      "MAX_AGE": "24",
      "POSITIONS": "DE,DT,NT,DL,EDGE,LB,ILB,OLB,MLB,CB,DB,S,FS,SS,SAF,NB",

      "SNAP_COUNTS": "1",
      "TEAM_DELAY_MS": "450",
      "HTTP_MAX_ATTEMPTS": "7",
      "HTTP_RETRY_BASE_MS": "400",
      "HTTP_RETRY_MAX_MS": "6000",
      "HTTP_COOLDOWN_MS": "9000",
      "HTTP_FINAL_COOLDOWN_MS": "15000",
      "PASS_MAX": "3",
      "SHUFFLE_TEAMS": "1",
      "DEBUG": "1"
    }
  }'

  aws lambda invoke \
  --region us-west-2 \
  --function-name pfr-weekly-2024 \
  --payload '{"mode":"ingest_roster","season":"2024","team_chunk_total":2,"team_chunk_index":1}' \
  out1.json