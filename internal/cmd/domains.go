package cmd

type DomainsCmd struct {
	List    DomainsListCmd    `cmd:"" name:"list" aliases:"ls" help:"List domains"`
	Get     DomainsGetCmd     `cmd:"" name:"get" help:"Get domain details"`
	Create  DomainsCreateCmd  `cmd:"" name:"create" aliases:"add" help:"Create domain"`
	Delete  DomainsDeleteCmd  `cmd:"" name:"delete" aliases:"rm" help:"Delete domain"`
	Aliases DomainsAliasesCmd `cmd:"" name:"aliases" help:"Manage domain aliases"`
}

type DomainsAliasesCmd struct {
	List   DomainsAliasesListCmd   `cmd:"" name:"list" aliases:"ls" help:"List domain aliases"`
	Create DomainsAliasesCreateCmd `cmd:"" name:"create" aliases:"add" help:"Create domain alias"`
	Delete DomainsAliasesDeleteCmd `cmd:"" name:"delete" aliases:"rm" help:"Delete domain alias"`
}
