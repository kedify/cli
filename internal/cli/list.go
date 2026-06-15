package cli

// ListCmd groups list subcommands.
type ListCmd struct {
	Clusters ListClustersCmd `cmd:"" help:"List clusters."`
}
