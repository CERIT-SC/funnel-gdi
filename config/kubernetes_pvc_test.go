package config

import "testing"

func TestKubernetesValidatePVCMode(t *testing.T) {
	cases := []struct {
		name    string
		k       *Kubernetes
		wantErr bool
	}{
		{"empty defaults to full", &Kubernetes{}, false},
		{"full", &Kubernetes{PVCMode: PVCModeFull}, false},
		{"shared", &Kubernetes{PVCMode: PVCModeShared}, false},
		{"pvc with storage class", &Kubernetes{PVCMode: PVCModePVC, StorageClassName: "nfs-csi"}, false},
		{"pvc without storage class", &Kubernetes{PVCMode: PVCModePVC}, true},
		{"unknown mode", &Kubernetes{PVCMode: "bogus"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.k.ValidatePVCMode()
			if (err != nil) != c.wantErr {
				t.Errorf("ValidatePVCMode() error = %v, wantErr %v", err, c.wantErr)
			}
		})
	}
}

func TestKubernetesPVCModeResources(t *testing.T) {
	cases := []struct {
		mode       string
		createsPV  bool
		createsPVC bool
	}{
		{"", true, true},
		{PVCModeFull, true, true},
		{PVCModePVC, false, true},
		{PVCModeShared, false, false},
	}
	for _, c := range cases {
		k := &Kubernetes{PVCMode: c.mode}
		if got := k.CreatesPV(); got != c.createsPV {
			t.Errorf("mode %q: CreatesPV() = %v, want %v", c.mode, got, c.createsPV)
		}
		if got := k.CreatesPVC(); got != c.createsPVC {
			t.Errorf("mode %q: CreatesPVC() = %v, want %v", c.mode, got, c.createsPVC)
		}
	}
}

func TestHTSGETAndSDAStorageValid(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"", false},
		{"ftp://example.org/", false},
		{"https://example.org/sda/", true},
		{"http://htsget.svc:8080/", true},
	}
	for _, c := range cases {
		if got := (&HTSGETStorage{ServiceURL: c.url}).Valid(); got != c.want {
			t.Errorf("HTSGETStorage{%q}.Valid() = %v, want %v", c.url, got, c.want)
		}
		if got := (&SDAStorage{ServiceURL: c.url}).Valid(); got != c.want {
			t.Errorf("SDAStorage{%q}.Valid() = %v, want %v", c.url, got, c.want)
		}
	}
}
