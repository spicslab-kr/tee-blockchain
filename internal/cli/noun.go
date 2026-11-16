package cli

// TEENoun is what the output calls the attested TEE for a document format: an enclave on AWS Nitro Enclaves (and when
// no format is given, since aws-nitro is every command's default), a guest on Intel TDX and AMD SEV-SNP, where the TEE
// is a whole confidential VM.
func TEENoun(format string) string {
	if format == "" || format == "aws-nitro" {
		return "enclave"
	}
	return "guest"
}
