// Package aws is this role's adapter to internal/setup/build/aws, which builds a software Dockerfile into an AWS Nitro
// enclave image (EIF) and its PCR0, PCR1 and PCR2. The work is done once there; this file only converts the input and
// the result, as internal/tee/parse/<role>/aws.go does for the Nitro parser. The Vendor and auditor copies are
// deliberately the same (STRUCTURE.md, "Why role code is copied per role").
package aws

import (
	"context"
	"io"

	"attestamp/internal/setup/build"
	core "attestamp/internal/setup/build/aws"
)

// Build is the input to one build; see internal/setup/build/aws for each field.
type Build struct {
	AppDir        string // directory holding the software's Dockerfile (build). Set exactly one of AppDir and DockerImage
	DockerImage   string // a docker image the user already built (pack)
	ImageName     string // image and EIF name
	Port          int    // vsock port the responder listens on inside the enclave. 0 means 5000
	ResponderPath string // the att-ra-responder to layer in; empty: the one next to the executable
	OutDir        string // result directory. Defaults to out
	Verify        bool   // build once more without cache and check that the PCRs match

	// AfterInstall, if set, gets the docker and aws-nitro-enclaves-cli versions right after they are installed, and an
	// error from it stops the build.
	AfterInstall func(build.NitroTools) error
}

// Result is what goes into the measurement record.
type Result struct {
	Name         string
	Image        string              // docker image tag with the responder layered in
	EIF          string              // enclave image file
	Measurements string              // raw nitro-cli build-enclave output (json)
	PCR          map[string]string   // PCR0, PCR1, PCR2
	Responder    build.ResponderInfo // the layered-in att-ra-responder
}

// Run builds; see internal/setup/build/aws.
func (b Build) Run(ctx context.Context, log io.Writer) (Result, error) {
	r, err := core.Build{
		AppDir: b.AppDir, DockerImage: b.DockerImage, ImageName: b.ImageName, Port: b.Port,
		ResponderPath: b.ResponderPath, OutDir: b.OutDir, Verify: b.Verify, AfterInstall: b.AfterInstall,
	}.Run(ctx, log)
	if err != nil {
		return Result{}, err
	}
	return Result{Name: r.Name, Image: r.Image, EIF: r.EIF, Measurements: r.Measurements, PCR: r.PCR, Responder: r.Responder}, nil
}
