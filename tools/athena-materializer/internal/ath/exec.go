package ath

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/athena"
	"github.com/aws/aws-sdk-go-v2/service/athena/types"
)

type Runner struct {
	Client    *athena.Client
	Workgroup string
	Database  string
	OutputS3  string // s3://bucket/prefix/
	Logger    *log.Logger
}

func (r *Runner) ExecAndWait(ctx context.Context, sql string) (*types.QueryExecution, error) {
	startOut, err := r.Client.StartQueryExecution(ctx, &athena.StartQueryExecutionInput{
		QueryString: &sql,
		QueryExecutionContext: &types.QueryExecutionContext{
			Database: &r.Database,
		},
		ResultConfiguration: &types.ResultConfiguration{
			OutputLocation: &r.OutputS3,
		},
		WorkGroup: &r.Workgroup,
	})
	if err != nil {
		return nil, fmt.Errorf("start query: %w", err)
	}
	qid := *startOut.QueryExecutionId
	if r.Logger != nil {
		r.Logger.Printf("athena: qid=%s started", qid)
	}

	tick := time.NewTicker(1 * time.Second)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-tick.C:
			ge, err := r.Client.GetQueryExecution(ctx, &athena.GetQueryExecutionInput{
				QueryExecutionId: &qid,
			})
			if err != nil {
				return nil, fmt.Errorf("get query execution: %w", err)
			}
			switch ge.QueryExecution.Status.State {
			case types.QueryExecutionStateSucceeded:
				if r.Logger != nil && ge.QueryExecution.Statistics != nil {
					stats := ge.QueryExecution.Statistics

					var scannedMB float64
					if stats.DataScannedInBytes != nil {
						scannedMB = float64(*stats.DataScannedInBytes) / 1024.0 / 1024.0
					}

					var execSec float64
					if stats.EngineExecutionTimeInMillis != nil {
						execSec = float64(*stats.EngineExecutionTimeInMillis) / 1000.0
					}

					r.Logger.Printf(
						"athena: qid=%s SUCCEEDED (data scanned=%.3f MB, exec=%.2fs)",
						qid, scannedMB, execSec,
					)
				}
				return ge.QueryExecution, nil
			case types.QueryExecutionStateFailed:
				msg := ""
				if ge.QueryExecution.Status.StateChangeReason != nil {
					msg = *ge.QueryExecution.Status.StateChangeReason
				}
				return nil, errors.New("athena failed: " + msg)
			case types.QueryExecutionStateCancelled:
				return nil, errors.New("athena cancelled")
			default:
				// still running
			}
		}
	}
}

func (r *Runner) CountRows(ctx context.Context, table string) (int64, error) {
	sql := fmt.Sprintf("SELECT COUNT(*) AS c FROM %s", table)
	exec, err := r.ExecAndWait(ctx, sql)
	if err != nil {
		return 0, err
	}
	gr, err := r.Client.GetQueryResults(ctx, &athena.GetQueryResultsInput{
		QueryExecutionId: exec.QueryExecutionId,
	})
	if err != nil {
		return 0, fmt.Errorf("get results: %w", err)
	}
	if len(gr.ResultSet.Rows) < 2 || len(gr.ResultSet.Rows[1].Data) < 1 || gr.ResultSet.Rows[1].Data[0].VarCharValue == nil {
		return 0, errors.New("unexpected COUNT(*) result shape")
	}
	var n int64
	if _, err := fmt.Sscan(*gr.ResultSet.Rows[1].Data[0].VarCharValue, &n); err != nil {
		return 0, fmt.Errorf("parse count: %w", err)
	}
	return n, nil
}
