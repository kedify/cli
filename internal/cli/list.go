package cli

func (c *ListClustersCmd) Run(ctx *context) error {
	token, err := resolveToken(ctx)
	if err != nil {
		return err
	}

	clusters, err := ctx.client.ListClusters(ctx.apiURL, token)
	if err != nil {
		return err
	}

	return ctx.writeOutput(ctx.stdout, clusters, c.Output)
}
