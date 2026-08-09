package report

import (
	"runtime"

	"github.com/emanuellcs/vpc-proof-agent/internal/buildinfo"
)

// agentName is the tool name reported in evidence documents.
const agentName = "vpc-proof"

// AgentInfoFromRuntime builds the agent metadata from build-time and runtime
// information.
func AgentInfoFromRuntime() AgentInfo {
	return AgentInfo{
		Name:      agentName,
		Version:   buildinfo.Version,
		Commit:    buildinfo.Commit,
		BuildDate: buildinfo.BuildDate,
		GoVersion: runtime.Version(),
		Platform:  runtime.GOOS + "/" + runtime.GOARCH,
		Developer: buildinfo.Developer,
	}
}
