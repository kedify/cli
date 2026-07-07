package service

type ClusterService interface {
	ListClusters(apiURL, token string) ([]map[string]any, error)
	GetCluster(apiURL, token, clusterID string) (map[string]any, error)
	GetRecommendations(apiURL, token, clusterID string) (any, error)
}
