package digitalocean

import (
	"errors"
	"os"
	"time"

	"github.com/digitalocean/go-metadata"
	"github.com/digitalocean/godo"
	"golang.org/x/net/context"
	"golang.org/x/oauth2"
)

type TokenAuth struct {
	AuthToken string
}

func (t *TokenAuth) Token() (*oauth2.Token, error) {
	token := &oauth2.Token{
		AccessToken: t.AuthToken,
	}
	return token, nil
}

type Client struct {
	client   *godo.Client
	metadata *metadata.Metadata

	region string
	id     int
}

func NewClient() (*Client, error) {
	mdata, err := metadata.NewClient().Metadata()
	if err != nil {
		return nil, err
	}

	token, err := fetchToken()
	if err != nil {
		return nil, err
	}

	auth := &TokenAuth{AuthToken: token}
	oauthClient := oauth2.NewClient(oauth2.NoContext, auth)
	gd := godo.NewClient(oauthClient)

	client := &Client{
		client:   gd,
		metadata: mdata,
		id:       mdata.DropletID,
		region:   mdata.Region,
	}
	return client, nil
}

func fetchToken() (string, error) {
	token := os.Getenv("DO_TOKEN")
	if token == "" {
		return token, errors.New("no token found")
	}
	return token, nil
}

func (c *Client) GetVolume(id string) (*godo.Volume, error) {
	vol, _, err := c.client.Storage.GetVolume(id)
	return vol, err
}

func (c *Client) CreateVolume(name string, size int64) (string, error) {
	size = size / GB

	req := &godo.VolumeCreateRequest{
		Region:        c.region,
		Name:          name,
		SizeGigaBytes: size,
	}

	vol, _, err := c.client.Storage.CreateVolume(req)
	if err != nil {
		return "", err
	}
	return vol.ID, nil
}

func (c *Client) DeleteVolume(name string) error {
	_, err := c.client.Storage.DeleteVolume(name)
	return err
}

func (c *Client) AttachVolume(id string) error {
	event, _, err := c.client.StorageActions.Attach(id, c.id)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	return c.waitUntilDone(ctx, event.ID)
}

func (c *Client) DetachVolume(id string) error {
	event, _, err := c.client.StorageActions.Detach(id)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	return c.waitUntilDone(ctx, event.ID)
}

func (c *Client) waitUntilDone(ctx context.Context, id int) error {
	for {
		action, _, err := c.client.Actions.Get(id)
		if err != nil {
			return errors.New("unable to fetch action information")
		}
		switch action.Status {
		case "completed":
			return nil
		case "errored":
			return errors.New("attach event failed")
		}
		select {
		case <-ctx.Done():
			return errors.New("attach event deadline exceeded")
		case <-time.After(5 * time.Second):
		}
	}
}
