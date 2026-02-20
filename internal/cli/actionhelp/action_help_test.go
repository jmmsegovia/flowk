package actionhelp

import (
	"strings"
	"testing"

	_ "flowk/internal/actions/auth/oauth2"
	_ "flowk/internal/actions/core/evaluate"
	_ "flowk/internal/actions/core/forloop"
	_ "flowk/internal/actions/core/parallel"
	_ "flowk/internal/actions/core/print"
	_ "flowk/internal/actions/core/sleep"
	_ "flowk/internal/actions/core/variables"
	_ "flowk/internal/actions/db/cassandra"
	_ "flowk/internal/actions/db/postgres"
	_ "flowk/internal/actions/infra/helm"
	_ "flowk/internal/actions/infra/kubernetes"
	_ "flowk/internal/actions/network/httpclient"
	_ "flowk/internal/actions/network/ssh"
	_ "flowk/internal/actions/network/telnet"
	"flowk/internal/actions/registry"
	_ "flowk/internal/actions/storage/gcloudstorage"
	_ "flowk/internal/actions/system/base64"
	_ "flowk/internal/actions/system/shell"
)

func TestBuildProvidesEnglishDescriptionsAndExample(t *testing.T) {
	help, err := Build("for")
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if !strings.Contains(help, "id — Unique identifier for the task within the flow.") {
		t.Fatalf("help output missing description for id: %s", help)
	}

	if !strings.Contains(help, "Example:\n") {
		t.Fatalf("help output missing example section: %s", help)
	}

	if !strings.Contains(help, "\"action\": \"FOR\"") {
		t.Fatalf("example output missing action placeholder: %s", help)
	}

	if !strings.Contains(help, "\"tasks\": [") {
		t.Fatalf("example output missing tasks array: %s", help)
	}
}

func TestPrintHelpMarksDescriptionOptional(t *testing.T) {
	help, err := Build("PRINT")
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if strings.Contains(help, "Required fields:\n  - description") {
		t.Fatalf("expected description to be optional for PRINT: %s", help)
	}

	expected := "\n  - description — string; Task description"
	if !strings.Contains(help, expected) {
		t.Fatalf("optional fields section missing description entry: %s", help)
	}
}

func TestIndexListsRegisteredActions(t *testing.T) {
	output := Index("flowk")

	if !strings.Contains(output, "Available actions:") {
		t.Fatalf("index output missing header: %s", output)
	}

	if !strings.Contains(output, "- FOR") {
		t.Fatalf("index output missing FOR action: %s", output)
	}

	if !strings.Contains(output, "flowk help action <name>") {
		t.Fatalf("index output missing usage hint: %s", output)
	}
}

func TestBuildForKubernetesIncludesConditionalSections(t *testing.T) {
	help, err := Build("KUBERNETES")
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	requiredSections := []string{
		"Action KUBERNETES",
		"Allowed values:\n  - action — \"KUBERNETES\"",
		"Required fields (depending on \"operation\"):",
		"1) operation = \"PORT_FORWARD\"",
		"2) operation = \"STOP_PORT_FORWARD\"",
		"3) operation = \"SCALE\"",
		"4) operation = \"GET_PODS\"",
		"5) operation = \"GET_DEPLOYMENTS\"",
		"6) operation = \"GET_LOGS\"",
		"7) operation = \"WAIT_FOR_POD_READINESS\"",
		"8) operation = any other value (default case)",
		"Examples:\n\n1) operation = \"PORT_FORWARD\"",
		"2) operation = \"STOP_PORT_FORWARD\"",
		"3) operation = \"SCALE\"",
		"4) operation = \"GET_PODS\"",
		"5) operation = \"GET_DEPLOYMENTS\"",
		"6) operation = \"GET_LOGS\"",
		"7) operation = \"WAIT_FOR_POD_READINESS\"",
		"8) operation = any other value (default case)",
	}

	for _, section := range requiredSections {
		if !strings.Contains(help, section) {
			t.Fatalf("expected help output to contain %q, got: %s", section, help)
		}
	}

	if !strings.Contains(help, "(note: \"context\" is NOT required)") {
		t.Fatalf("expected help output to mention context is not required in STOP_PORT_FORWARD case: %s", help)
	}

	expectedSnippets := []string{
		"\"id\": \"port-forward-task\"",
		"\"service\": \"<service-name>\"",
		"\"local_port\": \"<local-port>\"",
		"\"id\": \"stop-port-forward-task\"",
		"\"id\": \"scale-deployment-task\"",
		"\"deployments\": \"<deployment-names>\"",
		"\"id\": \"wait-for-pod-readiness\"",
		"\"max_wait_seconds\": \"<max-wait-seconds>\"",
		"\"poll_interval_seconds\": \"<poll-interval-seconds>\"",
		"\"id\": \"generic-k8s-task\"",
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(help, snippet) {
			t.Fatalf("expected help output to contain snippet %q, got: %s", snippet, help)
		}
	}
}

func TestBuildAllActionsAvoidsEscapedPlaceholders(t *testing.T) {
	for _, name := range registry.Names() {
		help, err := Build(name)
		if err != nil {
			t.Fatalf("Build(%s) error = %v", name, err)
		}

		if strings.Contains(help, "\\u003c") {
			t.Fatalf("help output for %s contains escaped placeholder: %s", name, help)
		}

		if !strings.Contains(help, "Action "+strings.ToUpper(name)) {
			t.Fatalf("help output for %s missing action header: %s", name, help)
		}
	}
}

func TestBuildDocumentationIncludesOperationsAndExample(t *testing.T) {
	doc, err := BuildDocumentation("kubernetes")
	if err != nil {
		t.Fatalf("BuildDocumentation() error = %v", err)
	}

	if strings.TrimSpace(doc.Example) == "" {
		t.Fatalf("expected non-empty example for kubernetes action")
	}

	operations := make(map[string]struct{})
	for _, op := range doc.Operations {
		operations[op.Name] = struct{}{}
	}

	expected := []string{"PORT_FORWARD", "STOP_PORT_FORWARD", "SCALE", "WAIT_FOR_POD_READINESS"}
	for _, name := range expected {
		if _, ok := operations[name]; !ok {
			t.Fatalf("expected operation %q in documentation, got %v", name, operations)
		}
	}
}

func TestFormatGuideMarkdownIncludesPrimerAndActions(t *testing.T) {
	guide, err := BuildGuide()
	if err != nil {
		t.Fatalf("BuildGuide() error = %v", err)
	}

	content := FormatGuideMarkdown(guide)
	if !strings.Contains(content, "action and operation catalog") {
		t.Fatalf("guide markdown missing primer header: %s", content)
	}

	if !strings.Contains(content, "KUBERNETES") {
		t.Fatalf("guide markdown missing at least one action name: %s", content)
	}
}

func TestBuildForHelmIncludesKeyOperations(t *testing.T) {
	help, err := Build("HELM")
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	required := []string{
		"Action HELM",
		"REPO_ADD",
		"UPGRADE_INSTALL",
		"ROLLBACK",
		"LINT",
	}
	for _, fragment := range required {
		if !strings.Contains(help, fragment) {
			t.Fatalf("expected help output to contain %q, got: %s", fragment, help)
		}
	}
}
