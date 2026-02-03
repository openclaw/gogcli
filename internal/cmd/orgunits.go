package cmd

type OrgunitsCmd struct {
	List   OrgunitsListCmd   `cmd:"" name:"list" aliases:"ls" help:"List organizational units"`
	Get    OrgunitsGetCmd    `cmd:"" name:"get" help:"Get organizational unit"`
	Create OrgunitsCreateCmd `cmd:"" name:"create" aliases:"add" help:"Create organizational unit"`
	Update OrgunitsUpdateCmd `cmd:"" name:"update" help:"Update organizational unit"`
	Delete OrgunitsDeleteCmd `cmd:"" name:"delete" aliases:"rm" help:"Delete organizational unit"`
}
