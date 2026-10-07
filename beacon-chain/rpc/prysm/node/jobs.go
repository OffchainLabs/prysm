package node

import (
	"net/http"
	"strconv"
	"time"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	"github.com/OffchainLabs/prysm/v7/runtime/jobs"
)

// GetJobs returns the status and progress of all tracked long-running node operations.
func (s *Server) GetJobs(w http.ResponseWriter, r *http.Request) {
	_, span := trace.StartSpan(r.Context(), "node.GetJobs")
	defer span.End()

	list := s.JobsRegistry.List()
	data := make([]*structs.JobData, 0, len(list))
	for _, j := range list {
		data = append(data, jobData(j))
	}
	httputil.WriteJson(w, &structs.GetJobsResponse{Data: data})
}

// GetJob returns the status and progress of the long-running node operation with the given job id.
func (s *Server) GetJob(w http.ResponseWriter, r *http.Request) {
	_, span := trace.StartSpan(r.Context(), "node.GetJob")
	defer span.End()

	id := r.PathValue("job_id")
	if id == "" {
		httputil.HandleError(w, "job_id is required in URL params", http.StatusBadRequest)
		return
	}
	j, ok := s.JobsRegistry.Get(id)
	if !ok {
		httputil.HandleError(w, "Job not found: "+id, http.StatusNotFound)
		return
	}
	httputil.WriteJson(w, &structs.GetJobResponse{Data: jobData(j)})
}

func jobData(j jobs.Job) *structs.JobData {
	d := &structs.JobData{
		Id:     j.ID,
		Status: string(j.Status),
		Phase:  j.Phase,
		Error:  j.Error,
	}
	if !j.StartedAt.IsZero() {
		d.StartedAt = j.StartedAt.UTC().Format(time.RFC3339)
	}
	if !j.UpdatedAt.IsZero() {
		d.UpdatedAt = j.UpdatedAt.UTC().Format(time.RFC3339)
	}
	if !j.FinishedAt.IsZero() {
		d.FinishedAt = j.FinishedAt.UTC().Format(time.RFC3339)
	}
	if j.Progress != nil {
		d.Progress = &structs.JobProgress{
			Current: strconv.FormatUint(j.Progress.Current, 10),
			Total:   strconv.FormatUint(j.Progress.Total, 10),
			Units:   j.Progress.Units,
		}
	}
	return d
}
