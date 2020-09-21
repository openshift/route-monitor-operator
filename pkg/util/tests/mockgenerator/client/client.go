package client

//go:generate mockgen -destination ../../generated/mocks/status-writer.go -package mocks sigs.k8s.io/controller-runtime/pkg/client StatusWriter
//go:generate mockgen -destination ../../generated/mocks/cr-client.go -package mocks sigs.k8s.io/controller-runtime/pkg/client Client
