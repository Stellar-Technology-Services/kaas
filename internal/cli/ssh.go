package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/Stellar-Technology-Services/kaas/internal/state"
	"github.com/spf13/cobra"
)

func newCPCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cp <source> <destination>",
		Short: "Copy files between the host and one sandbox with scp",
		Long:  "Copy a file to or from one sandbox. Prefix the remote side with a sandbox name, for example:\n  kaas cp my-app:~/workspace/app/report.xml ./\n  kaas cp ./patch.diff my-app:~/workspace/app/",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			source, destination := args[0], args[1]
			sourceName, sourcePath, sourceRemote := parseSandboxPath(source)
			destinationName, destinationPath, destinationRemote := parseSandboxPath(destination)
			if sourceRemote == destinationRemote {
				return fmt.Errorf("exactly one cp path must use sandbox:path syntax")
			}
			name := sourceName
			if destinationRemote {
				name = destinationName
			}
			if name == "" {
				return fmt.Errorf("remote path must be prefixed with a sandbox name, e.g. my-app:~/file")
			}
			store, _, err := openStore(cmd)
			if err != nil {
				return err
			}
			defer store.Close()
			sb, err := loadSandbox(store, name)
			if err != nil {
				return err
			}
			user, host, key := sandboxSSH(*sb)
			remote := fmt.Sprintf("%s@%s:%s", user, host, shellEscapeRemotePath(sourcePath))
			local := destinationPath
			if destinationRemote {
				remote, local = fmt.Sprintf("%s@%s:%s", user, host, shellEscapeRemotePath(destinationPath)), sourcePath
			}
			scpArgs := []string{"-i", key, "-P", strconv.Itoa(sb.SSHPort), "-o", "IdentitiesOnly=yes", "-o", "StrictHostKeyChecking=accept-new"}
			if sourceRemote {
				scpArgs = append(scpArgs, remote, local)
			} else {
				scpArgs = append(scpArgs, local, remote)
			}
			bin, err := exec.LookPath("scp")
			if err != nil {
				return fmt.Errorf("scp not found on PATH")
			}
			copyCmd := exec.Command(bin, scpArgs...)
			copyCmd.Stdin, copyCmd.Stdout, copyCmd.Stderr = os.Stdin, commandOutput(cmd), os.Stderr
			if err := copyCmd.Run(); err != nil {
				return fmt.Errorf("scp: %w", err)
			}
			printStyled(cmd, styleSuccess, "Copied %s → %s\n", source, destination)
			return nil
		},
	}
}

func parseSandboxPath(value string) (name, path string, remote bool) {
	prefix, rest, found := strings.Cut(value, ":")
	if !found || prefix == "" || strings.Contains(prefix, "/") || strings.HasPrefix(value, "./") {
		return "", value, false
	}
	return prefix, rest, true
}

func shellEscapeRemotePath(path string) string {
	// scp hands the remote component to an SSH shell. Single quoting preserves
	// spaces and metacharacters while deliberately leaving ~/ expandable.
	if strings.HasPrefix(path, "~/") {
		return path
	}
	return "'" + strings.ReplaceAll(path, "'", "'\\\"'\\\"'") + "'"
}

func sandboxSSH(sb state.Sandbox) (user, host, key string) {
	user = sb.SSHUser
	if user == "" {
		user = os.Getenv("USER")
	}
	host = sb.SSHHost
	if host == "" {
		host = "127.0.0.1"
	}
	key = sb.SSHKey
	if key == "" {
		key = os.ExpandEnv("$HOME/.colima/_lima/_config/user")
	}
	return
}

func newSSHCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "ssh [name]",
		Aliases: []string{"shell"},
		Short:   "SSH into a sandbox VM",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, _, err := openStore(cmd)
			if err != nil {
				return err
			}
			defer store.Close()

			name, err := resolveSandboxName(cmd, store, args)
			if err != nil {
				return err
			}
			sb, err := loadSandbox(store, name)
			if err != nil {
				return err
			}

			user, host, key := sandboxSSH(*sb)

			sshArgs := []string{
				"-i", key,
				"-p", strconv.Itoa(sb.SSHPort),
				"-o", "IdentitiesOnly=yes",
				"-o", "StrictHostKeyChecking=accept-new",
				fmt.Sprintf("%s@%s", user, host),
			}
			// Append any extra args after --
			if dash := cmd.ArgsLenAtDash(); dash >= 0 {
				sshArgs = append(sshArgs, args[dash:]...)
			}

			bin, err := exec.LookPath("ssh")
			if err != nil {
				return fmt.Errorf("ssh not found on PATH")
			}
			sshCmd := exec.Command(bin, sshArgs...)
			sshCmd.Stdin = os.Stdin
			sshCmd.Stdout = commandOutput(cmd)
			sshCmd.Stderr = os.Stderr
			if err := sshCmd.Run(); err != nil {
				return err
			}
			return nil
		},
	}
}
