# Nebraska Update Controller

[Nebraska](https://github.com/kinvolk/nebraska/) is an update management and monitoring service, for omaha-based updates.

This Kubernetes controller updates Kubernetes applications by talking to Nebraska server using the Nebraska's updater library in a controlled fashion.

The user needs to install the application using Flux's HelmRelease CR and then provide the GitRepository or HelmRepository in Nebraska's package.

This project is created as a proof-of-concept for providing managed updates to applications deployed on Kubernetes, and is therefore not intended for production at the moment.

## Contributing

Please check out the [contributing](./CONTRIBUTING.md) file, and observe our [Code of Conduct](./CODE_OF_CONDUCT.md) when participating in this project.

## License

 This project is released under the [MIT license](./LICENSE.txt).
