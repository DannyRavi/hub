package helm

import (
	"context"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/artifacthub/hub/internal/hub"
	"github.com/artifacthub/hub/internal/img"
	"github.com/artifacthub/hub/internal/pkg"
	"github.com/artifacthub/hub/internal/tests"
	"github.com/artifacthub/hub/internal/tracker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/time/rate"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/repo"
)

func TestWorker(t *testing.T) {
	logoImageURL := "http://icon.url"

	t.Run("handle register job", func(t *testing.T) {
		pkg1V1 := &repo.ChartVersion{
			Metadata: &chart.Metadata{
				Name:    "pkg1",
				Version: "1.0.0",
			},
			URLs: []string{
				"http://tests/pkg1-1.0.0.tgz",
			},
		}
		pkg2V1 := &repo.ChartVersion{
			Metadata: &chart.Metadata{
				Name:    "pkg2",
				Version: "1.0.0",
			},
			URLs: []string{
				"http://tests/pkg2-1.0.0.tgz",
			},
		}
		job := &Job{
			Kind:         Register,
			ChartVersion: pkg1V1,
			StoreLogo:    true,
		}

		t.Run("error downloading chart", func(t *testing.T) {
			t.Parallel()

			// Setup worker and expectations
			ww := newWorkerWrapper(context.Background())
			ww.queue <- job
			close(ww.queue)
			req, _ := http.NewRequest("GET", job.ChartVersion.URLs[0], nil)
			ww.hc.On("Do", req).Return(nil, tests.ErrFake)
			ww.ec.On("Append", ww.w.r.RepositoryID, mock.Anything).Return()

			// Run worker and check expectations
			ww.w.Run(ww.wg, ww.queue)
			ww.assertExpectations(t)
		})

		t.Run("error downloading chart (deprecated chart)", func(t *testing.T) {
			t.Parallel()

			// Setup worker and expectations
			ww := newWorkerWrapper(context.Background())
			job := &Job{
				Kind: Register,
				ChartVersion: &repo.ChartVersion{
					Metadata: &chart.Metadata{
						Name:       "pkg1",
						Version:    "1.0.0",
						Deprecated: true,
					},
					URLs: []string{
						"http://tests/pkg1-1.0.0.tgz",
					},
				},
				StoreLogo: true,
			}
			ww.queue <- job
			close(ww.queue)
			req, _ := http.NewRequest("GET", job.ChartVersion.URLs[0], nil)
			ww.hc.On("Do", req).Return(nil, tests.ErrFake)

			// Run worker and check expectations
			ww.w.Run(ww.wg, ww.queue)
			ww.assertExpectations(t)
		})

		t.Run("unexpected status downloading chart", func(t *testing.T) {
			t.Parallel()

			// Setup worker and expectations
			ww := newWorkerWrapper(context.Background())
			ww.queue <- job
			close(ww.queue)
			req, _ := http.NewRequest("GET", job.ChartVersion.URLs[0], nil)
			ww.hc.On("Do", req).Return(&http.Response{
				Body:       ioutil.NopCloser(strings.NewReader("")),
				StatusCode: http.StatusNotFound,
			}, nil)
			ww.ec.On("Append", ww.w.r.RepositoryID, mock.Anything).Return()

			// Run worker and check expectations
			ww.w.Run(ww.wg, ww.queue)
			ww.assertExpectations(t)
		})

		t.Run("error downloading logo image", func(t *testing.T) {
			t.Parallel()

			// Setup worker and expectations
			ww := newWorkerWrapper(context.Background())
			ww.queue <- job
			close(ww.queue)
			f, _ := os.Open("testdata/" + path.Base(job.ChartVersion.URLs[0]))
			reqChart, _ := http.NewRequest("GET", job.ChartVersion.URLs[0], nil)
			ww.hc.On("Do", reqChart).Return(&http.Response{
				Body:       f,
				StatusCode: http.StatusOK,
			}, nil)
			reqLogo, _ := http.NewRequest("GET", logoImageURL, nil)
			ww.hc.On("Do", reqLogo).Return(nil, tests.ErrFake)
			reqProv, _ := http.NewRequest("GET", job.ChartVersion.URLs[0]+".prov", nil)
			ww.hc.On("Do", reqProv).Return(&http.Response{
				Body:       ioutil.NopCloser(strings.NewReader("")),
				StatusCode: http.StatusNotFound,
			}, nil)
			ww.ec.On("Append", ww.w.r.RepositoryID, mock.Anything).Return()
			ww.pm.On("Register", mock.Anything, mock.Anything).Return(nil)

			// Run worker and check expectations
			ww.w.Run(ww.wg, ww.queue)
			ww.assertExpectations(t)
		})

		t.Run("unexpected status downloading logo image", func(t *testing.T) {
			t.Parallel()

			// Setup worker and expectations
			ww := newWorkerWrapper(context.Background())
			ww.queue <- job
			close(ww.queue)
			f, _ := os.Open("testdata/" + path.Base(job.ChartVersion.URLs[0]))
			reqChart, _ := http.NewRequest("GET", job.ChartVersion.URLs[0], nil)
			ww.hc.On("Do", reqChart).Return(&http.Response{
				Body:       f,
				StatusCode: http.StatusOK,
			}, nil)
			reqLogo, _ := http.NewRequest("GET", logoImageURL, nil)
			ww.hc.On("Do", reqLogo).Return(&http.Response{
				Body:       ioutil.NopCloser(strings.NewReader("")),
				StatusCode: http.StatusUnauthorized,
			}, nil)
			reqProv, _ := http.NewRequest("GET", job.ChartVersion.URLs[0]+".prov", nil)
			ww.hc.On("Do", reqProv).Return(&http.Response{
				Body:       ioutil.NopCloser(strings.NewReader("")),
				StatusCode: http.StatusNotFound,
			}, nil)
			ww.ec.On("Append", ww.w.r.RepositoryID, mock.Anything).Return()
			ww.pm.On("Register", mock.Anything, mock.Anything).Return(nil)

			// Run worker and check expectations
			ww.w.Run(ww.wg, ww.queue)
			ww.assertExpectations(t)
		})

		t.Run("error saving logo image", func(t *testing.T) {
			t.Parallel()

			// Setup worker and expectations
			ww := newWorkerWrapper(context.Background())
			ww.queue <- job
			close(ww.queue)
			f, _ := os.Open("testdata/" + path.Base(job.ChartVersion.URLs[0]))
			reqChart, _ := http.NewRequest("GET", job.ChartVersion.URLs[0], nil)
			ww.hc.On("Do", reqChart).Return(&http.Response{
				Body:       f,
				StatusCode: http.StatusOK,
			}, nil)
			reqLogo, _ := http.NewRequest("GET", logoImageURL, nil)
			ww.hc.On("Do", reqLogo).Return(&http.Response{
				Body:       ioutil.NopCloser(strings.NewReader("imageData")),
				StatusCode: http.StatusOK,
			}, nil)
			ww.ec.On("Append", ww.w.r.RepositoryID, mock.Anything).Return()
			reqProv, _ := http.NewRequest("GET", job.ChartVersion.URLs[0]+".prov", nil)
			ww.hc.On("Do", reqProv).Return(&http.Response{
				Body:       ioutil.NopCloser(strings.NewReader("")),
				StatusCode: http.StatusNotFound,
			}, nil)
			ww.is.On("SaveImage", mock.Anything, []byte("imageData")).Return("", tests.ErrFake)
			ww.pm.On("Register", mock.Anything, mock.Anything).Return(nil)

			// Run worker and check expectations
			ww.w.Run(ww.wg, ww.queue)
			ww.assertExpectations(t)
		})

		t.Run("error registering package", func(t *testing.T) {
			t.Parallel()

			// Setup worker and expectations
			ww := newWorkerWrapper(context.Background())
			ww.queue <- job
			close(ww.queue)
			f, _ := os.Open("testdata/" + path.Base(job.ChartVersion.URLs[0]))
			reqChart, _ := http.NewRequest("GET", job.ChartVersion.URLs[0], nil)
			ww.hc.On("Do", reqChart).Return(&http.Response{
				Body:       f,
				StatusCode: http.StatusOK,
			}, nil)
			reqLogo, _ := http.NewRequest("GET", logoImageURL, nil)
			ww.hc.On("Do", reqLogo).Return(&http.Response{
				Body:       ioutil.NopCloser(strings.NewReader("imageData")),
				StatusCode: http.StatusOK,
			}, nil)
			reqProv, _ := http.NewRequest("GET", job.ChartVersion.URLs[0]+".prov", nil)
			ww.hc.On("Do", reqProv).Return(&http.Response{
				Body:       ioutil.NopCloser(strings.NewReader("")),
				StatusCode: http.StatusNotFound,
			}, nil)
			ww.is.On("SaveImage", mock.Anything, []byte("imageData")).Return("imageID", nil)
			ww.pm.On("Register", mock.Anything, mock.Anything).Return(tests.ErrFake)
			ww.ec.On("Append", ww.w.r.RepositoryID, mock.Anything).Return()

			// Run worker and check expectations
			ww.w.Run(ww.wg, ww.queue)
			ww.assertExpectations(t)
		})

		t.Run("package registered successfully", func(t *testing.T) {
			t.Parallel()

			// Setup worker and expectations
			ww := newWorkerWrapper(context.Background())
			ww.queue <- job
			close(ww.queue)
			f, _ := os.Open("testdata/" + path.Base(job.ChartVersion.URLs[0]))
			reqChart, _ := http.NewRequest("GET", job.ChartVersion.URLs[0], nil)
			ww.hc.On("Do", reqChart).Return(&http.Response{
				Body:       f,
				StatusCode: http.StatusOK,
			}, nil)
			reqLogo, _ := http.NewRequest("GET", logoImageURL, nil)
			ww.hc.On("Do", reqLogo).Return(&http.Response{
				Body:       ioutil.NopCloser(strings.NewReader("imageData")),
				StatusCode: http.StatusOK,
			}, nil)
			reqProv, _ := http.NewRequest("GET", job.ChartVersion.URLs[0]+".prov", nil)
			ww.hc.On("Do", reqProv).Return(&http.Response{
				Body:       ioutil.NopCloser(strings.NewReader("")),
				StatusCode: http.StatusNotFound,
			}, nil)
			ww.is.On("SaveImage", mock.Anything, []byte("imageData")).Return("imageID", nil)
			ww.pm.On("Register", mock.Anything, &hub.Package{
				Name:        "pkg1",
				LogoURL:     "http://icon.url",
				LogoImageID: "imageID",
				IsOperator:  true,
				Description: "Package1 chart",
				License:     "Apache-2.0",
				Links: []*hub.Link{
					{
						Name: "link1",
						URL:  "https://link1.url",
					},
					{
						Name: "link2",
						URL:  "https://link2.url",
					},
				},
				Capabilities: "Basic Install",
				CRDs: []interface{}{
					map[string]interface{}{
						"kind":        "MyKind",
						"version":     "v1",
						"name":        "mykind",
						"displayName": "My Kind",
						"description": "Some nice description",
					},
				},
				CRDsExamples: []interface{}{
					map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "MyKind",
						"metadata": map[string]interface{}{
							"name": "mykind",
						},
						"spec": map[string]interface{}{
							"replicas": 1,
						},
					},
				},
				Version:    "1.0.0",
				AppVersion: "1.0.0",
				ContentURL: "http://tests/pkg1-1.0.0.tgz",
				Maintainers: []*hub.Maintainer{
					{
						Name:  "me-updated",
						Email: "me@me.com",
					},
					{
						Name:  "me2",
						Email: "me2@me.com",
					},
				},
				ContainersImages: []*hub.ContainerImage{
					{
						Name:  "img1",
						Image: "repo/img1:1.0.0",
					},
					{
						Name:        "img2",
						Image:       "repo/img2:2.0.0",
						Whitelisted: true,
					},
				},
				Changes: []string{
					"Added cool feature",
					"Fixed minor bug",
				},
				ContainsSecurityUpdates: true,
				Prerelease:              true,
				Repository: &hub.Repository{
					RepositoryID: "repo1",
				},
				CreatedAt: 0,
			}).Return(nil)

			// Run worker and check expectations
			ww.w.Run(ww.wg, ww.queue)
			ww.assertExpectations(t)
		})

		t.Run("package with logo in data url registered successfully", func(t *testing.T) {
			t.Parallel()

			// Setup worker and expectations
			ww := newWorkerWrapper(context.Background())
			job := &Job{
				Kind:         Register,
				ChartVersion: pkg2V1,
				StoreLogo:    true,
			}
			ww.queue <- job
			close(ww.queue)
			f, _ := os.Open("testdata/" + path.Base(job.ChartVersion.URLs[0]))
			reqChart, _ := http.NewRequest("GET", job.ChartVersion.URLs[0], nil)
			ww.hc.On("Do", reqChart).Return(&http.Response{
				Body:       f,
				StatusCode: http.StatusOK,
			}, nil)
			reqProv, _ := http.NewRequest("GET", job.ChartVersion.URLs[0]+".prov", nil)
			ww.hc.On("Do", reqProv).Return(&http.Response{
				Body:       ioutil.NopCloser(strings.NewReader("")),
				StatusCode: http.StatusNotFound,
			}, nil)
			expectedLogoData, _ := ioutil.ReadFile("testdata/red-dot.png")
			ww.is.On("SaveImage", mock.Anything, expectedLogoData).Return("imageID", nil)
			ww.pm.On("Register", mock.Anything, mock.Anything).Return(nil)

			// Run worker and check expectations
			ww.w.Run(ww.wg, ww.queue)
			ww.assertExpectations(t)
		})
	})

	t.Run("handle unregister job", func(t *testing.T) {
		job := &Job{
			Kind: Unregister,
			ChartVersion: &repo.ChartVersion{
				Metadata: &chart.Metadata{
					Name:    "pkg1",
					Version: "1.0.0",
				},
			},
		}

		t.Run("error unregistering package", func(t *testing.T) {
			t.Parallel()

			// Setup worker and expectations
			ww := newWorkerWrapper(context.Background())
			ww.queue <- job
			close(ww.queue)
			ww.pm.On("Unregister", mock.Anything, mock.Anything).Return(tests.ErrFake)
			ww.ec.On("Append", ww.w.r.RepositoryID, mock.Anything).Return()

			// Run worker and check expectations
			ww.w.Run(ww.wg, ww.queue)
			ww.assertExpectations(t)
		})

		t.Run("package unregistered successfully", func(t *testing.T) {
			t.Parallel()

			// Setup worker and expectations
			ww := newWorkerWrapper(context.Background())
			ww.queue <- job
			close(ww.queue)
			ww.pm.On("Unregister", mock.Anything, mock.Anything).Return(nil)

			// Run worker and check expectations
			ww.w.Run(ww.wg, ww.queue)
			ww.assertExpectations(t)
		})
	})
}

func TestEnrichPackageFromAnnotations(t *testing.T) {
	testCases := []struct {
		pkg            *hub.Package
		annotations    map[string]string
		expectedPkg    *hub.Package
		expectedErrMsg string
	}{
		// Changes
		{
			&hub.Package{},
			map[string]string{
				changesAnnotation: `
- Added cool feature
- Fixed minor bug
`,
			},
			&hub.Package{
				Changes: []string{
					"Added cool feature",
					"Fixed minor bug",
				},
			},
			"",
		},
		// CRDs
		{
			&hub.Package{},
			map[string]string{
				crdsAnnotation: `
- kind: MyKind
  version: v1
  name: mykind
  displayName: My Kind
  description: Some nice description
`,
			},
			&hub.Package{
				CRDs: []interface{}{
					map[string]interface{}{
						"description": "Some nice description",
						"displayName": "My Kind",
						"kind":        "MyKind",
						"name":        "mykind",
						"version":     "v1",
					},
				},
			},
			"",
		},
		// CRDs examples
		{
			&hub.Package{},
			map[string]string{
				crdsExamplesAnnotation: `
- apiVersion: v1
  kind: MyKind
  metadata:
    name: mykind
  spec:
    replicas: 1
`,
			},
			&hub.Package{
				CRDsExamples: []interface{}{
					map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "MyKind",
						"metadata": map[string]interface{}{
							"name": "mykind",
						},
						"spec": map[string]interface{}{
							"replicas": 1,
						},
					},
				},
			},
			"",
		},
		// Images
		{
			&hub.Package{},
			map[string]string{
				imagesAnnotation: `
- name: img1
  image: repo/img1:1.0.0
- name: img2
  image: repo/img2:2.0.0
  whitelisted: true
`,
			},
			&hub.Package{
				ContainersImages: []*hub.ContainerImage{
					{
						Name:  "img1",
						Image: "repo/img1:1.0.0",
					},
					{
						Name:        "img2",
						Image:       "repo/img2:2.0.0",
						Whitelisted: true,
					},
				},
			},
			"",
		},
		// License
		{
			&hub.Package{},
			map[string]string{
				licenseAnnotation: "Apache-2.0",
			},
			&hub.Package{
				License: "Apache-2.0",
			},
			"",
		},
		{
			&hub.Package{
				License: "GPL-3",
			},
			map[string]string{
				licenseAnnotation: "Apache-2.0",
			},
			&hub.Package{
				License: "Apache-2.0",
			},
			"",
		},
		{
			&hub.Package{
				License: "Apache-2.0",
			},
			map[string]string{
				licenseAnnotation: "",
			},
			&hub.Package{
				License: "Apache-2.0",
			},
			"",
		},
		// Links
		{
			&hub.Package{},
			map[string]string{
				linksAnnotation: `"{\"`,
			},
			&hub.Package{},
			"invalid links value",
		},
		{
			&hub.Package{
				Links: []*hub.Link{
					{
						Name: "",
						URL:  "https://link1.url",
					},
				},
			},
			map[string]string{
				linksAnnotation: `"{\"`,
			},
			&hub.Package{
				Links: []*hub.Link{
					{
						Name: "",
						URL:  "https://link1.url",
					},
				},
			},
			"invalid links value",
		},
		{
			&hub.Package{},
			map[string]string{
				linksAnnotation: `
- name: link1
  url: https://link1.url
`,
			},
			&hub.Package{
				Links: []*hub.Link{
					{
						Name: "link1",
						URL:  "https://link1.url",
					},
				},
			},
			"",
		},
		{
			&hub.Package{
				Links: []*hub.Link{
					{
						Name: "",
						URL:  "https://link1.url",
					},
				},
			},
			map[string]string{
				linksAnnotation: `
- name: link1
  url: https://link1.url
- name: link2
  url: https://link2.url
`,
			},
			&hub.Package{
				Links: []*hub.Link{
					{
						Name: "link1",
						URL:  "https://link1.url",
					},
					{
						Name: "link2",
						URL:  "https://link2.url",
					},
				},
			},
			"",
		},
		// Maintainers
		{
			&hub.Package{},
			map[string]string{
				maintainersAnnotation: `"{\"`,
			},
			&hub.Package{},
			"invalid maintainers value",
		},
		{
			&hub.Package{
				Maintainers: []*hub.Maintainer{
					{
						Name:  "user1",
						Email: "user1@email.com",
					},
				},
			},
			map[string]string{
				maintainersAnnotation: `"{\"`,
			},
			&hub.Package{
				Maintainers: []*hub.Maintainer{
					{
						Name:  "user1",
						Email: "user1@email.com",
					},
				},
			},
			"invalid maintainers value",
		},
		{
			&hub.Package{},
			map[string]string{
				maintainersAnnotation: `
- name: user1
  email: user1@email.com
`,
			},
			&hub.Package{
				Maintainers: []*hub.Maintainer{
					{
						Name:  "user1",
						Email: "user1@email.com",
					},
				},
			},
			"",
		},
		{
			&hub.Package{
				Maintainers: []*hub.Maintainer{
					{
						Name:  "user1",
						Email: "user1@email.com",
					},
				},
			},
			map[string]string{
				maintainersAnnotation: `
- name: user1-updated
  email: user1@email.com
- name: user2
  email: user2@email.com
`,
			},
			&hub.Package{
				Maintainers: []*hub.Maintainer{
					{
						Name:  "user1-updated",
						Email: "user1@email.com",
					},
					{
						Name:  "user2",
						Email: "user2@email.com",
					},
				},
			},
			"",
		},
		// Operator flag
		{
			&hub.Package{},
			map[string]string{
				operatorAnnotation: "invalid",
			},
			&hub.Package{},
			"invalid operator value",
		},
		{
			&hub.Package{},
			map[string]string{
				operatorAnnotation: "true",
			},
			&hub.Package{
				IsOperator: true,
			},
			"",
		},
		{
			&hub.Package{
				IsOperator: true,
			},
			map[string]string{
				operatorAnnotation: "false",
			},
			&hub.Package{
				IsOperator: false,
			},
			"",
		},
		{
			&hub.Package{
				IsOperator: true,
			},
			map[string]string{},
			&hub.Package{
				IsOperator: true,
			},
			"",
		},
		// Operator capabilities
		{
			&hub.Package{},
			map[string]string{
				operatorCapabilitiesAnnotation: "Basic Install",
			},
			&hub.Package{
				Capabilities: "Basic Install",
			},
			"",
		},
		// Prerelease
		{
			&hub.Package{},
			map[string]string{
				prereleaseAnnotation: "invalid",
			},
			&hub.Package{},
			"invalid prerelease value",
		},
		{
			&hub.Package{},
			map[string]string{
				prereleaseAnnotation: "true",
			},
			&hub.Package{
				Prerelease: true,
			},
			"",
		},
		{
			&hub.Package{
				Prerelease: true,
			},
			map[string]string{
				prereleaseAnnotation: "false",
			},
			&hub.Package{
				Prerelease: false,
			},
			"",
		},
		{
			&hub.Package{
				Prerelease: true,
			},
			map[string]string{},
			&hub.Package{
				Prerelease: true,
			},
			"",
		},
		// Security updates
		{
			&hub.Package{},
			map[string]string{
				securityUpdatesAnnotation: "true",
			},
			&hub.Package{
				ContainsSecurityUpdates: true,
			},
			"",
		},
	}
	for i, tc := range testCases {
		tc := tc
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()
			err := enrichPackageFromAnnotations(tc.pkg, tc.annotations)
			if tc.expectedErrMsg != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrMsg)
			} else {
				assert.Nil(t, err)
			}
			assert.Equal(t, tc.expectedPkg, tc.pkg)
		})
	}
}

type workerWrapper struct {
	wg    *sync.WaitGroup
	pm    *pkg.ManagerMock
	is    *img.StoreMock
	ec    *tracker.ErrorsCollectorMock
	hc    *tests.HTTPClientMock
	w     *Worker
	queue chan *Job
}

func newWorkerWrapper(ctx context.Context) *workerWrapper {
	// Setup worker
	pm := &pkg.ManagerMock{}
	is := &img.StoreMock{}
	ec := &tracker.ErrorsCollectorMock{}
	hc := &tests.HTTPClientMock{}
	r := &hub.Repository{RepositoryID: "repo1"}
	svc := &tracker.Services{
		Ctx:      ctx,
		Pm:       pm,
		Is:       is,
		Ec:       ec,
		Hc:       hc,
		GithubRL: rate.NewLimiter(rate.Inf, 0),
	}
	w := NewWorker(svc, r)
	queue := make(chan *Job, 100)

	// Wait group used for Worker.Run()
	var wg sync.WaitGroup
	wg.Add(1)

	return &workerWrapper{
		wg:    &wg,
		pm:    pm,
		is:    is,
		ec:    ec,
		hc:    hc,
		w:     w,
		queue: queue,
	}
}

func (ww *workerWrapper) assertExpectations(t *testing.T) {
	ww.wg.Wait()

	ww.pm.AssertExpectations(t)
	ww.is.AssertExpectations(t)
	ww.ec.AssertExpectations(t)
	ww.hc.AssertExpectations(t)
}
