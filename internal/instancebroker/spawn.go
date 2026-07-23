package instancebroker

import (
	"fmt"
	"os"
	"os/exec"
)

// internalBrokerEnvironment marks the child process that may participate in
// broker election and initialize authoritative mutable state.
const internalBrokerEnvironment = "OUTLOOK_MCP_INTERNAL_BROKER=1"

// IsBrokerProcess reports whether the current process was spawned as a private
// authoritative broker. It only reads the process environment.
func IsBrokerProcess() bool {
	return os.Getenv("OUTLOOK_MCP_INTERNAL_BROKER") == "1"
}

// SpawnBroker starts a detached copy of executable with the original public
// arguments and private broker environment marker. Its stdin and stdout are
// connected to the null device so it can never corrupt an MCP stdio stream.
func SpawnBroker(executable string, arguments []string) error {
	null, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("open null device for broker: %w", err)
	}
	defer null.Close()
	command := exec.Command(executable, arguments...)
	command.Env = append(os.Environ(), internalBrokerEnvironment)
	command.Stdin = null
	command.Stdout = null
	command.Stderr = os.Stderr
	if err := command.Start(); err != nil {
		return fmt.Errorf("start broker process: %w", err)
	}
	if err := command.Process.Release(); err != nil {
		return fmt.Errorf("release broker process: %w", err)
	}
	return nil
}
