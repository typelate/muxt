package load

import (
	"go/types"

	"golang.org/x/tools/go/packages"

	"github.com/typelate/muxt/internal/source"
)

// PackageWithReceiver reads the package at dir and the receiver type,
// nil when receiverType is empty. The receiver is read first, so a missing
// package is reported before a missing receiver -- both by Receiver -- and
// either before a templates variable that does not evaluate.
func PackageWithReceiver(dir string, pl []*packages.Package, receiverPackage, receiverType string, variables []string) (source.Package, *types.Named, error) {
	var receiver *types.Named
	if receiverType != "" {
		var err error
		if receiver, err = Receiver(dir, pl, receiverPackage, receiverType); err != nil {
			return source.Package{}, nil, err
		}
	}
	pkg, err := Package(dir, pl, variables)
	if err != nil {
		return source.Package{}, nil, err
	}
	return pkg, receiver, nil
}
