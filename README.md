# openshift-route-monitor-operator

![CodeQL](https://github.com/RiRa12621/openshift-route-monitor-operator/workflows/CodeQL/badge.svg?branch=master)
![Make Generate](https://github.com/RiRa12621/openshift-route-monitor-operator/workflows/Make%20Generate/badge.svg?branch=master)

## What does this do?
Automatically enables blackbox probes for routes on OpenShift clusters to be consumed by the Cluster Monitoring Operator
or any vanilla Prometheus Operator.

## How does this work?

### Exporter
The operator is making sure that here is one deployment of the [blackbox exporter](https://github.com/prometheus/blackbox_exporter).
If it does not exist in `openshift-monitoring`, it creates one.

### ServiceMonitors
The probes are effectively configured via `ServiceMonitors`.
openshift-route-monitor-operator creates `ServiceMonitors` based on the defined `RouteMonitors`.

### RouteMonitors
The operator watches all namespaces for `routeMonitors`.
They are used to define what route to probe.
`RouteMonitors` are namespace scoped and need to exist in the same namespaces as the `Route` they're used for.


## Caveats
Currently the blackbox exporter deployment is only using the default config file which only allows a limit set of probes.

## Contributing
Folow a simple workflow:
* Create Issue to explain what is wrong or missing
* Fork this repository
* Create a Pull Request referencing the issue you're adressing

This operator is build using the [operator-sdk](https://sdk.operatorframework.io)
However we automated away the need for you to run `make`.
On every Pull Request a GitHub action will trigger `make` to check if everything works as expected.

Once your PR is merged, an additional action will run make and check those changes in.

## ToDo

* [ ] add option to specify which probes to use
* [ ] add tests
