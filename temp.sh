aws lambda update-function-configuration \
  --function-name pfr-weekly-2024 \
  --environment "Variables={TABLE_NAME=defensive_players_2024,SEASON=2024,MAX_AGE=24,POSITIONS=DE,DT,NT,DL,EDGE,LB,ILB,OLB,MLB,CB,DB,S,FS,SS,SAF,NB,DEBUG=1,ASSUME_GS_EQUALS_G=1}"