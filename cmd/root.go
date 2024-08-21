package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/hierynomus/taipan"
	home "github.com/mitchellh/go-homedir"
	"github.com/rb3ckers/trafficmirror/internal/config"
	"github.com/rb3ckers/trafficmirror/internal/proxy"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
)

var (
	Version string
	Commit  string
	Date    string
)

var EnvPrefix = "TRAFFICMIRROR"

func RootCommand(cfg *config.Config) *cobra.Command {
	var verbosity int

	cmd := &cobra.Command{
		Use:   "trafficmirror",
		Short: "Runs Traffic Mirror",
		Long: `
HTTP proxy that:
* sends requests to a main endpoint from which the response is returned
* can mirror the requests to any additional number of endpoints

Additional targets are configured via PUT/DELETE on the '/targets?url=<endpoint>'.
`,
		Version: fmt.Sprintf("%s (Built on: %s, Commit: %s)", Version, Date, Commit),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			switch verbosity {
			case 0:
				// Nothing to do
			case 1:
				zerolog.SetGlobalLevel(zerolog.InfoLevel)
			case 2: //nolint:gomnd
				zerolog.SetGlobalLevel(zerolog.DebugLevel)
			default:
				zerolog.SetGlobalLevel(zerolog.TraceLevel)
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			PrintUsage(cfg)
			if err := RunProxy(cfg); err != nil {
				return err
			}

			return nil
		},
	}

	cmd.PersistentFlags().CountVarP(&verbosity, "verbose", "v", "Print more verbose logging")

	cmd.Flags().StringP("listen", "l", ":8080", "Address to listen on and mirror traffic from")
	cmd.Flags().StringP("main", "m", "http://localhost:8888", "Main proxy target, its responses will be returned to the client")
	cmd.Flags().String("targets", "targets", "Path on which additional targets to mirror to can be added/deleted/listed via PUT, DELETE and GET")
	cmd.Flags().String("targets-address", "", "Address on which the targets endpoint is made available. Leave empty to expose it on the address that is being mirrored")
	cmd.Flags().String("username", "", "Username to protect the configuration 'targets' endpoint with.")
	cmd.Flags().String("password", "", "Password to protect the configuration 'targets' endpoint with.")
	cmd.Flags().String("passwordFile", "", "Provide a file that contains username/password to protect the configuration 'targets' endpoint. Contains 1 username/password combination separated by ':'.")
	cmd.Flags().Int("fail-after", 30, "Remove a target when it has been failing for this many minutes.")                                                 //nolint:gomnd
	cmd.Flags().Int("max-queued-requests", 500, "Maximum amount of requests queued per mirror.")                                                         //nolint:gomnd
	cmd.Flags().Int("main-target-delay-ms", 0, "Delay delivery to main target, allowing slower mirrors to keep up and increase discovered parallelism.") //nolint:gomnd
	cmd.Flags().Int("retry-after", 1, "After 5 successive failures a target is temporarily disabled, it will be retried after this many minutes.")
	cmd.Flags().StringSlice("mirror", []string{}, "Start with mirroring traffic to provided targets")

	return cmd
}

func RunProxy(cfg *config.Config) error {
	sigs := make(chan os.Signal, 1)
	done := make(chan bool, 1)

	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	p := proxy.NewProxy(cfg)

	go func() {
		sig := <-sigs
		log.Printf("Received signal '%s', exiting\n", sig)

		if err := p.Stop(); err != nil {
			panic(err)
		}

		done <- true
	}()

	if err := p.Start(context.Background()); err != nil {
		return err
	}

	<-done

	return nil
}

func Execute(ctx context.Context) {
	cfg := &config.Config{}
	cmd := RootCommand(cfg)

	homeFolder, err := home.Expand("~/.trafficmirror")
	if err != nil {
		fmt.Printf("%s", err)
		os.Exit(1)
	}

	zerolog.SetGlobalLevel(zerolog.ErrorLevel)

	taipanConfig := &taipan.Config{
		DefaultConfigName:  "trafficmirror",
		ConfigurationPaths: []string{".", homeFolder},
		EnvironmentPrefix:  EnvPrefix,
		AddConfigFlag:      true,
		ConfigObject:       cfg,
		PrefixCommands:     true,
	}

	t := taipan.New(taipanConfig)
	t.Inject(cmd)

	if err := cmd.ExecuteContext(ctx); err != nil {
		fmt.Printf("🎃 %s\n", err)
		os.Exit(1)
	}
}

func PrintUsage(cfg *config.Config) {
	var targetsText string
	if cfg.TargetsListenAddress != "" {
		targetsText = fmt.Sprintf("http://%s/%s", cfg.TargetsListenAddress, cfg.TargetsEndpoint)
	} else {
		targetsText = fmt.Sprintf("http://%s/%s", cfg.ListenAddress, cfg.TargetsEndpoint)
	}

	fmt.Printf("Add/remove/list mirror targets via PUT/DELETE/GET at %s:\n", targetsText)
	fmt.Printf("List  : curl %s\n", targetsText)
	fmt.Printf("Add   : curl -X PUT %s?url=http://localhost:5678\n", targetsText)
	fmt.Printf("Remove: curl -X DELETE %s?url=http://localhost:5678\n", targetsText)
	fmt.Println()
}
