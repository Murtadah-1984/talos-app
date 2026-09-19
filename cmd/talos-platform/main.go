// Command talos-platform is the platform's CLI (§30). It is a thin REST
// client over the same API the web UI uses; it must never implement a
// second, independent business layer.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/talos-platform/talos-platform/internal/cli"
)

func main() {
	var apiURL, token string

	root := &cobra.Command{
		Use:   "talos-platform",
		Short: "CLI for the Talos Kubernetes Cluster Management Platform",
	}
	root.PersistentFlags().StringVar(&apiURL, "api-url", envOr("TALOS_PLATFORM_API_URL", "http://localhost:8080"), "Platform API base URL")
	root.PersistentFlags().StringVar(&token, "token", os.Getenv("TALOS_PLATFORM_TOKEN"), "Bearer token")

	newClient := func() *cli.Client { return cli.NewClient(apiURL, token) }

	root.AddCommand(clusterCmd(newClient), machineCmd(newClient), gitopsCmd(newClient), workflowCmd(newClient))

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func printJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func clusterCmd(newClient func() *cli.Client) *cobra.Command {
	cmd := &cobra.Command{Use: "cluster", Short: "Manage clusters"}

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List clusters",
		RunE: func(_ *cobra.Command, _ []string) error {
			var out any
			if err := newClient().Get("/api/v1/clusters", &out); err != nil {
				return err
			}
			printJSON(out)
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "get [id]",
		Short: "Get a cluster by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			var out any
			if err := newClient().Get("/api/v1/clusters/"+args[0], &out); err != nil {
				return err
			}
			printJSON(out)
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "plan [id]",
		Short: "Show the plan for provisioning a cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			var out any
			if err := newClient().Post("/api/v1/clusters/"+args[0]+"/plan", nil, &out); err != nil {
				return err
			}
			printJSON(out)
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "provision [id]",
		Short: "Provision a cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			var out any
			if err := newClient().Post("/api/v1/clusters/"+args[0]+"/provision", nil, &out); err != nil {
				return err
			}
			printJSON(out)
			return nil
		},
	})

	upgrade := &cobra.Command{
		Use:   "upgrade [id]",
		Short: "Upgrade a cluster's Kubernetes/Talos version",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			k8s, _ := c.Flags().GetString("kubernetes-version")
			talos, _ := c.Flags().GetString("talos-version")
			var out any
			body := map[string]string{"kubernetesVersion": k8s, "talosVersion": talos}
			if err := newClient().Post("/api/v1/clusters/"+args[0]+"/upgrade", body, &out); err != nil {
				return err
			}
			printJSON(out)
			return nil
		},
	}
	upgrade.Flags().String("kubernetes-version", "", "Target Kubernetes version")
	upgrade.Flags().String("talos-version", "", "Target Talos version")
	cmd.AddCommand(upgrade)

	return cmd
}

func machineCmd(newClient func() *cli.Client) *cobra.Command {
	cmd := &cobra.Command{Use: "machine", Short: "Manage machines"}

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List machines",
		RunE: func(_ *cobra.Command, _ []string) error {
			var out any
			if err := newClient().Get("/api/v1/machines", &out); err != nil {
				return err
			}
			printJSON(out)
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "health [id]",
		Short: "Show a machine's live Talos health",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			var out any
			if err := newClient().Get("/api/v1/machines/"+args[0]+"/health", &out); err != nil {
				return err
			}
			printJSON(out)
			return nil
		},
	})

	return cmd
}

func gitopsCmd(newClient func() *cli.Client) *cobra.Command {
	cmd := &cobra.Command{Use: "gitops", Short: "Inspect GitOps status"}

	cmd.AddCommand(&cobra.Command{
		Use:   "diff [cluster-id]",
		Short: "Show a cluster's GitOps/Argo CD status (change sets and Application health)",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			var out any
			if err := newClient().Get("/api/v1/clusters/"+args[0]+"/gitops", &out); err != nil {
				return err
			}
			printJSON(out)
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "sync [cluster-id]",
		Short: "Trigger an immediate Argo CD sync for a cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			var out any
			if err := newClient().Post("/api/v1/clusters/"+args[0]+"/sync", nil, &out); err != nil {
				return err
			}
			printJSON(out)
			return nil
		},
	})

	return cmd
}

func workflowCmd(newClient func() *cli.Client) *cobra.Command {
	cmd := &cobra.Command{Use: "workflow", Short: "Inspect workflows"}

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List workflows",
		RunE: func(_ *cobra.Command, _ []string) error {
			var out any
			if err := newClient().Get("/api/v1/workflows", &out); err != nil {
				return err
			}
			printJSON(out)
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "get [id]",
		Short: "Get a workflow and its steps",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			var out any
			if err := newClient().Get("/api/v1/workflows/"+args[0], &out); err != nil {
				return err
			}
			printJSON(out)
			return nil
		},
	})

	return cmd
}
