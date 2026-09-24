package accessidentity_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

const presentedToken = "eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJTWU4tT1BFUkFUT1ItMDEifQ.c2lnbmF0dXJl"

func TestOperatorCredentialNeverRendersTheToken(t *testing.T) {
	credential := accessidentity.NewOperatorCredential(presentedToken)

	var jsonLog, textLog bytes.Buffer
	slog.New(slog.NewJSONHandler(&jsonLog, nil)).Info("presented", "credential", credential)
	slog.New(slog.NewTextHandler(&textLog, nil)).Info("presented", "credential", credential)
	marshalled, err := json.Marshal(credential)
	if err != nil {
		t.Fatal(err)
	}
	renderings := map[string]string{
		"fmt verbs": fmt.Sprintf("%v|%+v|%#v|%s|%q|%x", credential, credential, credential, credential, credential, credential),
		"slog JSON": jsonLog.String(),
		"slog text": textLog.String(),
		"JSON":      string(marshalled),
		"error":     fmt.Errorf("verify %v: %w", credential, errors.New("boom")).Error(),
	}
	for name, rendered := range renderings {
		if strings.Contains(rendered, "eyJ") || strings.Contains(rendered, "c2lnbmF0dXJl") {
			t.Errorf("%s renders the token: %s", name, rendered)
		}
	}
	if credential.Token() != presentedToken {
		t.Fatalf("Token() = %q, want the presented token for the verifier", credential.Token())
	}
}

func TestUnconfiguredOperatorVerifierAnswersNotConfiguredToAnyPresentation(t *testing.T) {
	var verifier accessidentity.OperatorCredentialVerifier = accessidentity.UnconfiguredOperatorCredentialVerifier{}
	for _, token := range []string{presentedToken, ""} {
		if _, err := verifier.VerifyOperatorCredential(context.Background(), accessidentity.NewOperatorCredential(token)); !errors.Is(err, accessidentity.ErrAccessChannelNotConfigured) {
			t.Fatalf("token %q: err = %v, want ErrAccessChannelNotConfigured", token, err)
		}
	}
}
