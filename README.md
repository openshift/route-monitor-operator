# openshift-route-monitor-operator

## What does this do?
Automatically enables blackbox probes for routes on OpenShift clusters to be consumed by the Cluster Monitoring Operator
or any vanilla Prometheus Operator.

## How does this work?

### Exporter
The operator is making sure that there is one deployment + service of the [blackbox exporter](https://github.com/prometheus/blackbox_exporter).
If it does not exist in `openshift-monitoring`, it creates one.

### ServiceMonitors
The probes are effectively configured via `ServiceMonitors`, see more details in [Prometheus Operator troubleshooting docs](https://github.com/prometheus-operator/prometheus-operator/blob/566b18b2c9bf62ff3558804a69de5e1127ce8171/Documentation/user-guides/running-exporters.md#the-goal-of-servicemonitors).
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

## Development

In order to develop the repo follow these steps to get an env started:

1. run `make && make test` to test build and deploy
2. change [Makefile](./Makefile) and [config/manager/manager.yaml](config/manager/manager.yaml) to point to the repo you wish to use
3. build and deploy with `make docker-build && make docker-push`
4. use `make deploy` to deploy your operator on a cluster you are logged into
5. check logs with `kubectl logs -n openshift-logging deploy/route-monitor-operator-controller-manager -c manager`
6. retrigger pull of pod with `kubectl delete -n openshift-monitoring -lapp=route-monitor-operator,component=operator`

## ToDo

* [ ] add option to specify which probes to use
