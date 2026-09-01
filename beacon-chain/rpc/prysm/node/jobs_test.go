package node

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/runtime/jobs"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestGetJobs(t *testing.T) {
	t.Run("returns all jobs", func(t *testing.T) {
		registry := jobs.NewRegistry()
		backfillJob, err := registry.Register("backfill")
		require.NoError(t, err)
		backfillJob.Start()
		backfillJob.SetPhase("backfilling")
		backfillJob.SetProgress(150, 1000, "slots")
		otherJob, err := registry.Register("other-op")
		require.NoError(t, err)
		otherJob.Start()
		otherJob.Fail(errors.New("something broke"))

		s := &Server{JobsRegistry: registry}
		request := httptest.NewRequest(http.MethodGet, "/prysm/v1/node/jobs", nil)
		writer := httptest.NewRecorder()

		s.GetJobs(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetJobsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, 2, len(resp.Data))

		first := resp.Data[0]
		require.Equal(t, "backfill", first.Id)
		require.Equal(t, string(jobs.StatusRunning), first.Status)
		require.Equal(t, "backfilling", first.Phase)
		require.NotEqual(t, "", first.StartedAt)
		require.NotEqual(t, "", first.UpdatedAt)
		require.Equal(t, "", first.FinishedAt)
		require.NotNil(t, first.Progress)
		require.Equal(t, "150", first.Progress.Current)
		require.Equal(t, "1000", first.Progress.Total)
		require.Equal(t, "slots", first.Progress.Units)

		second := resp.Data[1]
		require.Equal(t, "other-op", second.Id)
		require.Equal(t, string(jobs.StatusFailed), second.Status)
		require.Equal(t, "something broke", second.Error)
		require.NotEqual(t, "", second.FinishedAt)
	})

	t.Run("no registry returns empty list", func(t *testing.T) {
		s := &Server{}
		request := httptest.NewRequest(http.MethodGet, "/prysm/v1/node/jobs", nil)
		writer := httptest.NewRecorder()

		s.GetJobs(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetJobsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.NotNil(t, resp.Data)
		require.Equal(t, 0, len(resp.Data))
	})
}

func TestGetJob(t *testing.T) {
	registry := jobs.NewRegistry()
	backfillJob, err := registry.Register("backfill")
	require.NoError(t, err)
	backfillJob.Start()
	backfillJob.SetPhase("backfilling")
	backfillJob.SetProgress(150, 1000, "slots")
	s := &Server{JobsRegistry: registry}

	t.Run("ok", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/prysm/v1/node/jobs/backfill", nil)
		request.SetPathValue("job_id", "backfill")
		writer := httptest.NewRecorder()

		s.GetJob(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetJobResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.NotNil(t, resp.Data)
		require.Equal(t, "backfill", resp.Data.Id)
		require.Equal(t, string(jobs.StatusRunning), resp.Data.Status)
		require.Equal(t, "backfilling", resp.Data.Phase)
		require.NotNil(t, resp.Data.Progress)
		require.Equal(t, "150", resp.Data.Progress.Current)
	})

	t.Run("not found", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/prysm/v1/node/jobs/unknown", nil)
		request.SetPathValue("job_id", "unknown")
		writer := httptest.NewRecorder()

		s.GetJob(writer, request)
		require.Equal(t, http.StatusNotFound, writer.Code)
		body, err := io.ReadAll(writer.Body)
		require.NoError(t, err)
		require.StringContains(t, "Job not found", string(body))
	})

	t.Run("no registry returns not found", func(t *testing.T) {
		noRegistry := &Server{}
		request := httptest.NewRequest(http.MethodGet, "/prysm/v1/node/jobs/backfill", nil)
		request.SetPathValue("job_id", "backfill")
		writer := httptest.NewRecorder()

		noRegistry.GetJob(writer, request)
		require.Equal(t, http.StatusNotFound, writer.Code)
	})
}
