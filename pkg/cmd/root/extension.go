package root

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/cli/cli/v2/pkg/extensions"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type ExternalCommandExitError struct {
	*exec.ExitError
}

type extensionReleaseInfo struct {
	CurrentVersion string
	LatestVersion  string
	Pinned         bool
	URL            string
}

func NewCmdExtension(io *iostreams.IOStreams, em extensions.ExtensionManager, ext extensions.Extension) *cobra.Command {
	updateMessageChan := make(chan *extensionReleaseInfo)
	cs := io.ColorScheme()

	return &cobra.Command{
		Use:   ext.Name(),
		Short: fmt.Sprintf("Extension %s", ext.Name()),
		// PreRun handles looking up whether extension has a latest version only when the command is ran.
		PreRun: func(c *cobra.Command, args []string) {
			go func() {
				if ext.UpdateAvailable() {
					updateMessageChan <- &extensionReleaseInfo{
						CurrentVersion: ext.CurrentVersion(),
						LatestVersion:  ext.LatestVersion(),
						Pinned:         ext.IsPinned(),
						URL:            ext.URL(),
					}
				}
			}()
		},
		RunE: func(c *cobra.Command, args []string) error {
			args = append([]string{ext.Name()}, args...)
			if _, err := em.Dispatch(args, io.In, io.Out, io.ErrOut); err != nil {
				var execError *exec.ExitError
				if errors.As(err, &execError) {
					return &ExternalCommandExitError{execError}
				}
				return fmt.Errorf("failed to run extension: %w\n", err)
			}
			return nil
		},
		// PostRun handles communicating extension release information if found
		PostRun: func(c *cobra.Command, args []string) {
			select {
			case releaseInfo := <-updateMessageChan:
				if releaseInfo != nil {
					stderr := io.ErrOut
					fmt.Fprintf(stderr, "\n\n%s %s → %s\n",
						cs.Yellowf("A new release of %s is available:", ext.Name()),
						cs.Cyan(strings.TrimPrefix(releaseInfo.CurrentVersion, "v")),
						cs.Cyan(strings.TrimPrefix(releaseInfo.LatestVersion, "v")))
					if releaseInfo.Pinned {
						fmt.Fprintf(stderr, "To upgrade, run: gh extension upgrade %s --force\n", ext.Name())
					} else {
						fmt.Fprintf(stderr, "To upgrade, run: gh extension upgrade %s\n", ext.Name())
					}
					fmt.Fprintf(stderr, "%s\n\n",
						cs.Yellow(releaseInfo.URL))
				}
			case <-time.After(1 * time.Second):
				// Bail on checking for new extension update as its taking too long
			}
		},
		GroupID: "extension",
		Annotations: map[string]string{
			"skipAuthCheck": "true",
		},
		DisableFlagParsing: true,
	}
}
