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

### ClusterUrlMonitors

The operator watches all namespaces for `ClusterUrlMonitors`.

They are used to define what URL to probe, based on the cluster domain to allow monitoring of URLs of applications deployed to the cluster,
which do not make use of a `Route` (i.e. the api server). A `ClusterUrlMonitor` consists of a `prefix`, a `port`, and a `suffix` which make up the probed URL as follows:

```
<prefix><cluster-domain>:<port><suffix>
```

Getting prefix and suffix right is in the users' responsibility.
In most cases the `prefix` will end with a `.` while the suffix will start with a `/` but this is not checked or fixed by the controller.
`ClusterUrlMonitors` are namespace scoped.

### Alerting
The operator implements  [Multiwindow, Multi-Burn-Rate Alerts](https://sre.google/workbook/alerting-on-slos/) in a unique way.

The official calculation for Multiwindow Multi-burn alerting will only successfully work on services and applications that already have
a sufficient baseline of metrics to work and calculate availability upon.
In a case of newly created services, alerts will fire immediately and will only be remediated once there's enough data present.
In some cases a user might want to create a Monitor for a newly created Route or ClusterUrl.
To support this, the operator [takes into account](https://github.com/openshift/route-monitor-operator/blob/c707066cf74b129a64e362fe4c3c99a7d7f36f88/pkg/util/templates/templates.go#L105) the overall number of existing probes, in a way that if there are no sufficient probes (yet), an alert will not fire.

## Caveats

Currently the blackbox exporter deployment is only using the default config file which only allows a limit set of probes.

## Development

In order to develop the repo follow these steps to get an env started:

1. run `make test` to test build and deploy
2. change [Makefile](./Makefile) and [config/manager/manager.yaml](config/manager/manager.yaml) to point to the repo you wish to use
3. build and deploy with `make docker-build docker-push`
    3.1. if you want to use a local image use IMG=<custom-image>
4. use `make deploy` to deploy your operator on a cluster you are logged into
    4.1. this also can have the IMG
5. check logs with `oc logs -n openshift-monitoring deploy/route-monitor-operator-controller-manager -c manager`
6. retrigger pull of pod with `oc delete -n openshift-monitoring -lapp=route-monitor-operator,component=operator`

### Test operator locally

The [makefile](./Makefile) has a command to run the operator locally:

```
make run
```

### Running integration tests

The integration test suite is located in `int/`. You can execute the test suite against a cluster you are currently logged into.
It will expose the registry of that cluster, push the operator to it and execute a suite of tests to ensure the operator does it's work.
Tested with running crc locally and being logged in as `kubeadmin`. If you want to run the tests as a different user,
set the `KUBEUSER` environment variable before executing the tests, and make sure you're logged in as that user to the cluster.

```
make test-integration
```

## ToDo

* [ ] add option to specify which probes to use
* [ ] make service monitor use a different interval via modifying a line in the spec of route monitor
