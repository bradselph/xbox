To install this:

```bash
# Install dependencies
go get github.com/google/gousb

# Build
go build

# Run with default settings (500Hz polling)
./xbox-controller

# Or specify custom polling frequency
./xbox-controller -freq 1000

# Enable debugging
./xbox-controller -debug 1
```

For Windows users, you'll need the libusb drivers installed. You can use Zadig (https://zadig.akeo.ie/) to install the drivers for your Xbox controller.