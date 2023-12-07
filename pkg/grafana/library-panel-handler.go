package grafana

import (
	"errors"
	"fmt"
	"path/filepath"

	"encoding/json"

	log "github.com/sirupsen/logrus"

	"github.com/grafana/grizzly/pkg/grizzly"
	"github.com/grafana/tanka/pkg/kubernetes/manifest"

	"github.com/grafana/grafana-openapi-client-go/client/library_elements"
	"github.com/grafana/grafana-openapi-client-go/models"
)

// LibraryPanelHandler is a Grizzly Handler for Grafana library panel
type LibraryPanelHandler struct {
	Provider grizzly.Provider
}

// NewLibraryPanelHandler returns a new Grizzly Handler for Grafana library panel resources
func NewLibraryPanelHandler(provider grizzly.Provider) *LibraryPanelHandler {
	return &LibraryPanelHandler{
		Provider: provider,
	}
}

// Kind returns the kind for this handler
func (h *LibraryPanelHandler) Kind() string {
	return "LibraryPanel"
}

// Validate returns the uid of resource
func (h *LibraryPanelHandler) Validate(resource grizzly.Resource) error {
	uid, exist := resource.GetSpecString("uid")
	if exist {
		if uid != resource.Name() {
			return fmt.Errorf("uid '%s' and name '%s', don't match", uid, resource.Name())
		}
	}
	return nil
}

// APIVersion returns group and version of the provider of this resource
func (h *LibraryPanelHandler) APIVersion() string {
	return h.Provider.APIVersion()
}

// GetExtension returns the file name extension for a library panel
func (h *LibraryPanelHandler) GetExtension() string {
	return "json"
}

const (
	libraryPanelGlob    = "library-panel/library-panel-*"
	libraryPanelPattern = "library-panel/library-panel-%s.%s"
)

// FindResourceFiles identifies files within a directory that this handler can process
func (h *LibraryPanelHandler) FindResourceFiles(dir string) ([]string, error) {
	path := filepath.Join(dir, libraryPanelGlob)
	return filepath.Glob(path)
}

// ResourceFilePath returns the location on disk where a resource should be updated
func (h *LibraryPanelHandler) ResourceFilePath(resource grizzly.Resource, filetype string) string {
	return fmt.Sprintf(libraryPanelPattern, resource.Name(), filetype)
}

// Parse parses a manifest object into a struct for this resource type
func (h *LibraryPanelHandler) Parse(m manifest.Manifest) (grizzly.Resources, error) {
	resource := grizzly.Resource(m)
	return grizzly.Resources{resource}, nil
}

// Unprepare removes unnecessary panels from a remote resource ready for presentation/comparison
func (h *LibraryPanelHandler) Unprepare(resource grizzly.Resource) *grizzly.Resource {
	resource.DeleteSpecKey("id")
	return &resource
}

// Prepare gets a resource ready for dispatch to the remote endpoint
func (h *LibraryPanelHandler) Prepare(existing, resource grizzly.Resource) *grizzly.Resource {
	return &resource
}

// GetUID returns the UID for a resource
func (h *LibraryPanelHandler) GetUID(resource grizzly.Resource) (string, error) {
	return resource.Name(), nil
}

// GetByUID retrieves JSON for a resource from an endpoint, by UID
func (h *LibraryPanelHandler) GetByUID(UID string) (*grizzly.Resource, error) {
	return h.getRemoteLibraryPanel(UID)
}

// GetRemote retrieves a datasource as a Resource
func (h *LibraryPanelHandler) GetRemote(resource grizzly.Resource) (*grizzly.Resource, error) {
	return h.GetByUID(resource.Name())
}

// ListRemote retrieves as list of UIDs of all remote resources
func (h *LibraryPanelHandler) ListRemote() ([]string, error) {
	return h.getRemoteLibraryPanelList()
}

// Add pushes a library panel to Grafana via the API
func (h *LibraryPanelHandler) Add(resource grizzly.Resource) error {
	return h.postLibraryPanel(resource)
}

// Update pushes a library panel to Grafana via the API
func (h *LibraryPanelHandler) Update(existing, resource grizzly.Resource) error {
	return h.putLibraryPanel(resource)
}

func (h *LibraryPanelHandler) getRemoteLibraryPanelList() ([]string, error) {
	params := library_elements.NewGetLibraryElementsParams()
	client, err := h.Provider.(ClientProvider).Client()
	if err != nil {
		return nil, err
	}

	panelsOk, err := client.LibraryElements.GetLibraryElements(params, nil)
	if err != nil {
		return nil, err
	}
	panels := panelsOk.GetPayload().Result
	log.Infof("LibraryPanel.getRemoteLibraryPanelList: found %d panels", panels.TotalCount)

	uids := make([]string, panels.TotalCount)
	for i, panel := range panels.Elements {
		log.Infof("LibraryPanel.getRemoteLibraryPanelList: - UID:%s", panel.UID)
		uids[i] = panel.UID
	}
	return uids, nil
}

func (h *LibraryPanelHandler) getRemoteLibraryPanel(uid string) (*grizzly.Resource, error) {
	client, err := h.Provider.(ClientProvider).Client()
	if err != nil {
		return nil, err
	}

	log.Infof("LibraryPanel.getRemoteLibraryPanel: UID:%s", uid)
	params := library_elements.NewGetLibraryElementByUIDParams().WithLibraryElementUID(uid)
	panelOk, err := client.LibraryElements.GetLibraryElementByUID(params, nil)
	if err != nil {
		var gErr *library_elements.GetLibraryElementByUIDNotFound
		if errors.As(err, &gErr) {
			log.Infof("LibraryPanel.getRemoteLibraryPanel: not found")
			return nil, grizzly.ErrNotFound
		} else {
			return nil, err
		}
	}

	var panel = panelOk.GetPayload()
	spec, err := structToMap(panel)
	if err != nil {
		return nil, err
	}

	resource := grizzly.NewResource(h.APIVersion(), h.Kind(), uid, spec)
	var jsonResource string
	jsonResource, _ = resource.SpecAsJSON()
	log.Infof(jsonResource)
	return &resource, nil
}

func (h *LibraryPanelHandler) postLibraryPanel(resource grizzly.Resource) error {
	data, err := json.Marshal(resource.Spec())
	if err != nil {
		return err
	}

	var panel models.CreateLibraryElementCommand
	err = json.Unmarshal(data, &panel)
	if err != nil {
		return err
	}
	// should not be needed
	panel.Name = resource.Name()
	panel.UID = resource.UID()
	panel.Model = resource.Spec()

	client, err := h.Provider.(ClientProvider).Client()
	if err != nil {
		return err
	}

	var jsonResource string
	jsonResource, _ = resource.SpecAsJSON()
	log.Infof(jsonResource)
	log.Infof("LibraryPanel.postLibraryPanel: Name:%s", panel.Name)
	log.Infof("LibraryPanel.postLibraryPanel: UID:%s", panel.UID)

	params := library_elements.NewCreateLibraryElementParams().WithBody(&panel)
	_, err = client.LibraryElements.CreateLibraryElement(params, nil)
	return err
}

func (h *LibraryPanelHandler) putLibraryPanel(resource grizzly.Resource) error {
	data, err := json.Marshal(resource.Spec())
	if err != nil {
		return err
	}

	var panel models.PatchLibraryElementCommand
	err = json.Unmarshal(data, &panel)
	if err != nil {
		return err
	}
	client, err := h.Provider.(ClientProvider).Client()
	if err != nil {
		return err
	}

	log.Infof("LibraryPanel.putLibraryPanel: UID:%s", panel.UID)
	params := library_elements.NewUpdateLibraryElementParams().WithLibraryElementUID(panel.UID).WithBody(&panel)
	_, err = client.LibraryElements.UpdateLibraryElement(params, nil, nil)
	return err
}
