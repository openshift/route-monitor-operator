package client

//go:generate mockgen -destination ../../generated/mocks/client/status-writer.go -package $GOPACKAGE sigs.k8s.io/controller-runtime/pkg/client  StatusWriter
//go:generate mockgen -destination ../../generated/mocks/client/cr-client.go -package $GOPACKAGE sigs.k8s.io/controller-runtime/pkg/client  Client
