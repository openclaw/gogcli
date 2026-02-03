package cmd

import "github.com/steipete/gogcli/internal/googleapi"

var (
	newVaultService   = googleapi.NewVault
	newStorageService = googleapi.NewStorage
)

type VaultCmd struct {
	Matters VaultMattersCmd `cmd:"" name:"matters" help:"Manage Vault matters"`
	Holds   VaultHoldsCmd   `cmd:"" name:"holds" help:"Manage Vault holds"`
	Exports VaultExportsCmd `cmd:"" name:"exports" help:"Manage Vault exports"`
}
