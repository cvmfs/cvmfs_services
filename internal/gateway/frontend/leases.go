package frontend

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	gw "github.com/cvmfs/gateway/internal/gateway"
	be "github.com/cvmfs/gateway/internal/gateway/backend"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

// MakeLeasesHandler creates an HTTP handler for the API root
func MakeLeasesHandler(services *be.Services) http.HandlerFunc {
	return func(w http.ResponseWriter, h *http.Request) {
		vs := mux.Vars(h)
		token, hasArg := vs["token"]
		switch h.Method {
		case "GET":
			handleGetLeases(services, token, w, h)
		case "POST":
			if hasArg {
				// Committing an existing lease (transaction)
				handleCommitLease(services, token, w, h)
			} else {
				// Requesting a new lease
				handleNewLease(services, w, h)
			}
		case "DELETE":
			handleCancelLease(services, token, w, h)
		default:
			reqID, _ := h.Context().Value(idKey).(uuid.UUID)
			gw.Log.Error().
				Str("component", "http").
				Str("req_id", reqID.String()).
				Msgf("invalid HTTP method: %v", h.Method)
			http.Error(w, "invalid method", http.StatusNotFound)
		}
	}
}

func handleGetLeases(services *be.Services, token string, w http.ResponseWriter, h *http.Request) {
	reqID, _ := h.Context().Value(idKey).(uuid.UUID)
	msg := make(map[string]interface{})
	if token == "" {
		leases, err := be.GetLeases(services)
		if err != nil {
			httpWrapError(&reqID, err, err.Error(), w, http.StatusInternalServerError)
		}
		msg["status"] = "ok"
		msg["data"] = leases
	} else {
		lease, err := be.GetLease(services, token)
		if err != nil {
			httpWrapError(&reqID, err, err.Error(), w, http.StatusInternalServerError)
		}
		msg["data"] = lease
	}

	t0, _ := h.Context().Value(t0Key).(time.Time)
	gw.Log.Debug().
		Str("component", "http").
		Str("req_id", reqID.String()).
		Float64("time", time.Since(t0).Seconds()).
		Msg("request processed")

	replyJSON(&reqID, w, msg)
}

func handleNewLease(services *be.Services, w http.ResponseWriter, h *http.Request) {
	reqID, _ := h.Context().Value(idKey).(uuid.UUID)

	var reqMsg struct {
		Path    string `json:"path"`
		Version string `json:"api_version"` // cvmfs_swissknife sends this field as a string
	}
	if err := json.NewDecoder(h.Body).Decode(&reqMsg); err != nil {
		httpWrapError(&reqID, err, "invalid request body", w, http.StatusBadRequest)
		return
	}

	clientVersion, _ := strconv.Atoi(reqMsg.Version)
	msg := make(map[string]interface{})
	if clientVersion < MinAPIProtocolVersion {
		msg["status"] = "error"
		msg["reason"] = fmt.Sprintf(
			"incompatible request version: %v, min version: %v",
			clientVersion,
			MinAPIProtocolVersion)
	} else {
		// The authorization is expected to have the correct format, since it has already been checked.
		keyID := strings.Split(h.Header.Get("Authorization"), " ")[0]
		token, err := be.NewLease(services, keyID, reqMsg.Path)
		if err != nil {
			if busyError, ok := err.(be.PathBusyError); ok {
				msg["status"] = "path_busy"
				msg["time_remaining"] = busyError.Remaining().String()
			} else {
				msg["status"] = "error"
				msg["reason"] = err.Error()
			}
		} else {
			msg["status"] = "ok"
			msg["session_token"] = token
			msg["max_api_version"] = MaxAPIVersion(clientVersion)
		}
	}

	t0, _ := h.Context().Value(t0Key).(time.Time)
	gw.Log.Debug().
		Str("component", "http").
		Str("req_id", reqID.String()).
		Float64("time", time.Since(t0).Seconds()).
		Msg("request processed")

	replyJSON(&reqID, w, msg)
}

func handleCommitLease(services *be.Services, token string, w http.ResponseWriter, h *http.Request) {
	reqID, _ := h.Context().Value(idKey).(uuid.UUID)

	var reqMsg struct {
		OldRootHash string `json:"old_root_hash"`
		NewRootHash string `json:"new_root_hash"`
		gw.RepositoryTag
	}
	if err := json.NewDecoder(h.Body).Decode(&reqMsg); err != nil {
		httpWrapError(&reqID, err, "invalid request body", w, http.StatusBadRequest)
		return
	}

	msg := make(map[string]interface{})
	if err := be.CommitLease(
		services, token, reqMsg.OldRootHash, reqMsg.NewRootHash, reqMsg.RepositoryTag); err != nil {
		msg["status"] = "error"
		msg["reason"] = err.Error()
	} else {
		msg["status"] = "ok"
	}

	t0, _ := h.Context().Value(t0Key).(time.Time)
	gw.Log.Debug().
		Str("component", "http").
		Str("req_id", reqID.String()).
		Float64("time", time.Since(t0).Seconds()).
		Msg("request processed")

	replyJSON(&reqID, w, msg)
}

func handleCancelLease(services *be.Services, token string, w http.ResponseWriter, h *http.Request) {
	if token == "" {
		http.Error(w, "missing token", http.StatusBadRequest)
		return
	}

	reqID, _ := h.Context().Value(idKey).(uuid.UUID)

	msg := make(map[string]interface{})

	if err := be.CancelLease(services, token); err != nil {
		msg["status"] = "error"
		if _, ok := err.(be.InvalidTokenError); ok {
			msg["reason"] = "invalid_token"
		} else {
			msg["reason"] = err.Error()
		}

	} else {
		msg["status"] = "ok"
	}

	t0, _ := h.Context().Value(t0Key).(time.Time)
	gw.Log.Debug().
		Str("component", "http").
		Str("req_id", reqID.String()).
		Float64("time", time.Since(t0).Seconds()).
		Msg("request processed")

	replyJSON(&reqID, w, msg)
}
