package cli

type ListRecommendationsCmd struct {
	ClusterID string `arg:"" name:"cluster-id" help:"Cluster id."`
	Output    string `name:"output" short:"o" help:"Output format." enum:"text,json,yaml" default:"text"`
}

func (c *ListRecommendationsCmd) Run(ctx *context) error {
	token, err := resolveToken(ctx)
	if err != nil {
		return err
	}

	recommendations, err := ctx.client.GetRecommendations(ctx.apiURL, token, c.ClusterID)
	if err != nil {
		return err
	}

	return ctx.writeOutput(ctx.stdout, recommendations, c.Output)
}
