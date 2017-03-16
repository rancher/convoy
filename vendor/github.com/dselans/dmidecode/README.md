dmidecode
=========

`dmidecode` is a Go library that parses the output of the `dmidecode` command
and makes it accessible via a simple map data structure.

In addition, the library exposes a couple of convenience methods for quicker record lookups.

## Usage

```go
import (
    dmidecode "github.com/dselans/dmidecode"
)

dmi := dmidecode.New()

if err := dmi.Run(); err != nil {
    fmt.Printf("Unable to get dmidecode information. Error: %v\n", err)
}

// You can search by record name
byNameData, byNameErr := dmi.SearchByName("System Information")

// or you can also search by record type
byTypeData, byTypeErr := dmi.SearchByType(1)

// or you can just access the data directly
for handle, record := range dmi.Data {
    fmt.Println("Checking record:", handle)
    for k, v := range record {
        fmt.Printf("Key: %v Val: %v\n", k, v)
    }
}
```

## Note
Record elements which contain an array/list, are stored as strings separated by 2 tabs (same as in `dmidecode` output). This may change in the future, but for the time being it's simple and quick.

Note that `dmidecode` requires root privs to run as the binary reads `/dev/mem`.
