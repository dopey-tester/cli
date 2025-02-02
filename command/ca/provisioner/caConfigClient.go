package provisioner

import (
	"context"

	"github.com/pkg/errors"
	"github.com/smallstep/certificates/authority"
	"github.com/smallstep/certificates/authority/config"
	"github.com/smallstep/certificates/authority/provisioner"
	"github.com/smallstep/certificates/ca"
	"go.step.sm/cli-utils/ui"
	"go.step.sm/linkedca"
)

// nodb implements the certificates/Adminclient interface with noops.
type nodb struct{}

func newNoDB() *nodb {
	return &nodb{}
}

func (n *nodb) CreateProvisioner(ctx context.Context, prov *linkedca.Provisioner) error {
	return nil
}

func (n *nodb) GetProvisioner(ctx context.Context, id string) (*linkedca.Provisioner, error) {
	//nolint:nilnil // nodb is a noop interface.
	return nil, nil
}

func (n *nodb) GetProvisioners(ctx context.Context) ([]*linkedca.Provisioner, error) {
	return nil, nil
}

func (n *nodb) UpdateProvisioner(ctx context.Context, prov *linkedca.Provisioner) error {
	return nil
}

func (n *nodb) DeleteProvisioner(ctx context.Context, id string) error {
	return nil
}

func (n *nodb) CreateAdmin(ctx context.Context, admin *linkedca.Admin) error {
	return nil
}

func (n *nodb) GetAdmin(ctx context.Context, id string) (*linkedca.Admin, error) {
	//nolint:nilnil // nodb is a noop interface.
	return nil, nil
}

func (n *nodb) GetAdmins(ctx context.Context) ([]*linkedca.Admin, error) {
	return nil, nil
}

func (n *nodb) UpdateAdmin(ctx context.Context, prov *linkedca.Admin) error {
	return nil
}

func (n *nodb) DeleteAdmin(ctx context.Context, id string) error {
	return nil
}

func (n *nodb) CreateAuthorityPolicy(ctx context.Context, policy *linkedca.Policy) error {
	return nil
}

func (n *nodb) GetAuthorityPolicy(ctx context.Context) (*linkedca.Policy, error) {
	//nolint:nilnil // nodb is a noop interface.
	return nil, nil
}

func (n *nodb) UpdateAuthorityPolicy(ctx context.Context, policy *linkedca.Policy) error {
	return nil
}

func (n *nodb) DeleteAuthorityPolicy(ctx context.Context) error {
	return nil
}

type caConfigClient struct {
	configFile string
	ctx        context.Context
	auth       *authority.Authority
}

func newCaConfigClient(ctx context.Context, cfg *config.Config, cfgFile string) (*caConfigClient, error) {
	provClxn := provisioner.NewCollection(provisioner.Audiences{})
	for _, p := range cfg.AuthorityConfig.Provisioners {
		if err := provClxn.Store(p); err != nil {
			return nil, err
		}
	}
	//nolint:contextcheck // no context for backward compatibility
	a, err := authority.New(cfg, authority.WithAdminDB(newNoDB()),
		//nolint:staticcheck // TODO: WithProvisioners has been deprecated, temporarily do not lint this line.
		authority.WithSkipInit(), authority.WithProvisioners(provClxn))
	if err != nil {
		return nil, errors.Wrapf(err, "error loading authority")
	}

	return &caConfigClient{
		configFile: cfgFile,
		ctx:        ctx,
		auth:       a,
	}, nil
}

func (client *caConfigClient) CreateProvisioner(prov *linkedca.Provisioner) (*linkedca.Provisioner, error) {
	if err := client.auth.StoreProvisioner(client.ctx, prov); err != nil {
		return nil, errors.Wrapf(err, "error storing provisioner")
	}

	if err := client.write(); err != nil {
		return nil, err
	}

	return prov, nil
}

func (client *caConfigClient) GetProvisioner(opts ...ca.ProvisionerOption) (*linkedca.Provisioner, error) {
	prov, err := client.loadProvisioner(opts...)
	if err != nil {
		return nil, err
	}
	linkedcaProv, err := authority.ProvisionerToLinkedca(prov)
	if err != nil {
		return nil, errors.Wrapf(err, "error converting provisioner interface to linkedca provisioner")
	}

	return linkedcaProv, nil
}

func (client *caConfigClient) UpdateProvisioner(name string, prov *linkedca.Provisioner) error {
	if err := client.auth.UpdateProvisioner(client.ctx, prov); err != nil {
		return errors.Wrapf(err, "error updating provisioner")
	}

	if err := client.write(); err != nil {
		return err
	}

	return nil
}

func (client *caConfigClient) RemoveProvisioner(opts ...ca.ProvisionerOption) error {
	prov, err := client.loadProvisioner(opts...)
	if err != nil {
		return err
	}
	if err := client.auth.RemoveProvisioner(client.ctx, prov.GetID()); err != nil {
		return errors.Wrapf(err, "error removing provisioner")
	}

	if err := client.write(); err != nil {
		return err
	}

	return nil
}

func (client *caConfigClient) loadProvisioner(opts ...ca.ProvisionerOption) (provisioner.Interface, error) {
	o := new(ca.ProvisionerOptions)
	if err := o.Apply(opts); err != nil {
		return nil, err
	}

	var (
		err  error
		prov provisioner.Interface
	)

	switch {
	case o.ID != "":
		prov, err = client.auth.LoadProvisionerByID(o.ID)
	case o.Name != "":
		prov, err = client.auth.LoadProvisionerByName(o.Name)
	default:
		return nil, errors.New("provisioner options must define either ID or Name to remove")
	}

	return prov, errors.Wrapf(err, "error loading provisioner")
}

func (client *caConfigClient) GetProvisioners(opts ...ca.ProvisionerOption) (provisioner.List, error) {
	o := new(ca.ProvisionerOptions)
	if err := o.Apply(opts); err != nil {
		return nil, err
	}

	if o.Limit == 0 {
		o.Limit = 100
	}

	var (
		cursor = o.Cursor
		limit  = o.Limit
		provs  = provisioner.List{}
	)
	for {
		page, nextCursor, err := client.auth.GetProvisioners(cursor, limit)
		if err != nil {
			return nil, err
		}
		provs = append(provs, page...)
		if nextCursor == "" {
			return provs, nil
		}
		cursor = nextCursor
	}
}

func (client *caConfigClient) write() error {
	provs, err := client.GetProvisioners()
	if err != nil {
		return err
	}
	cfg := client.auth.GetConfig()
	cfg.AuthorityConfig.Provisioners = provs
	if err := cfg.Save(client.configFile); err != nil {
		return err
	}

	ui.Println("Success! Your `step-ca` config has been updated. To pick up the new configuration SIGHUP (kill -1 <pid>) or restart the step-ca process.")

	return nil
}
