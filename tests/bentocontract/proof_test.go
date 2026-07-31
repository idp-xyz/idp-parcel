package bentocontract_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/tests/bentocontract"
)

const (
	// proofPathVariable requests a candidate proof. Without it the suite runs
	// as an ordinary test and writes nothing.
	proofPathVariable = "IDP_PARCEL_BENTO_PROOF"

	// dsnVariable gates the PostgreSQL contract cases. A proof written while it
	// is unset would claim integration coverage the run skipped.
	dsnVariable = "IDP_PARCEL_POSTGRES_DSN"
)

// proof is the minimal result the framework coordination job reads. Field names
// and order follow the framework JSON v1 contract; no business data, credential
// or failure detail may be added here.
type proof struct {
	FormatVersion        int    `json:"format_version"`
	Consumer             string `json:"consumer"`
	ConsumerCommit       string `json:"consumer_commit"`
	CandidateVersion     string `json:"candidate_version"`
	ModulePath           string `json:"module_path"`
	ModuleChecksum       string `json:"module_checksum"`
	ContractSuiteVersion string `json:"contract_suite_version"`
	Result               string `json:"result"`
	CompletedAt          string `json:"completed_at"`
}

// TestMain writes the proof only after every contract case passed, so the file
// can never claim a result this suite did not actually produce.
func TestMain(m *testing.M) {
	code := m.Run()
	path := os.Getenv(proofPathVariable)
	if path == "" {
		os.Exit(code)
	}
	if code != 0 {
		fmt.Fprintln(os.Stderr, "contract failed; no proof written")
		os.Exit(code)
	}
	if err := writeProof(path); err != nil {
		fmt.Fprintln(os.Stderr, "write candidate proof:", err)
		os.Exit(1)
	}
	os.Exit(code)
}

func writeProof(path string) error {
	if os.Getenv(dsnVariable) == "" {
		return fmt.Errorf("%s is required to write a proof; a run with skipped PostgreSQL cases is not a proof", dsnVariable)
	}
	commit, err := headCommit()
	if err != nil {
		return err
	}
	content, err := json.MarshalIndent(proof{
		FormatVersion:        1,
		Consumer:             "PARCEL",
		ConsumerCommit:       commit,
		CandidateVersion:     bentocontract.Version,
		ModulePath:           bentocontract.ModulePath,
		ModuleChecksum:       bentocontract.ModuleSum,
		ContractSuiteVersion: bentocontract.ContractSuiteVersion,
		Result:               "PASS",
		CompletedAt:          time.Now().UTC().Format(time.RFC3339Nano),
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(content, '\n'), 0o644)
}

// headCommit reports the commit the contract actually ran against. A dirty tree
// means the tested code is not the recorded commit, so the proof is refused.
func headCommit() (string, error) {
	status, err := exec.Command("git", "status", "--porcelain").Output()
	if err != nil {
		return "", fmt.Errorf("read repository status: %w", err)
	}
	if strings.TrimSpace(string(status)) != "" {
		return "", errors.New("the working tree is dirty; commit before producing a proof")
	}
	head, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return "", fmt.Errorf("read HEAD commit: %w", err)
	}
	return strings.TrimSpace(string(head)), nil
}
