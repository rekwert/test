package hypervisor

import "testing"

func TestMapServerPowerState_CompleteWithoutGuestAgent(t *testing.T) {
	s := &Server{Status: "complete", GuestAgentActive: false}
	if got := MapServerPowerState(s); got != "stopped" {
		t.Fatalf("MapServerPowerState(complete, ga=false) = %q, want stopped", got)
	}
}

func TestMapServerPowerState_CompleteWithGuestAgent(t *testing.T) {
	s := &Server{Status: "complete", GuestAgentActive: true}
	if got := MapServerPowerState(s); got != "running" {
		t.Fatalf("MapServerPowerState(complete, ga=true) = %q, want running", got)
	}
}

func TestServerPoweredOn_RemoteStateOff(t *testing.T) {
	s := &Server{
		Status:           "complete",
		GuestAgentActive: true,
		RemoteStateKnown: true,
		RemoteState:      false,
	}
	if ServerPoweredOn(s) {
		t.Fatal("remoteState=false should mean powered off even with stale guest agent")
	}
}

func TestServerReadyForGuestSetup_VFFailedWithOS(t *testing.T) {
	s := &Server{
		Status:           "failed",
		IP:               "66.248.206.61",
		OSImageVersionID: 23,
		OSBuilt:          true,
		BuildFailed:      true,
		GuestAgentActive: true,
	}
	if !ServerReadyForGuestSetup(s) {
		t.Fatal("failed commission with installed OS should continue guest setup")
	}
}

func TestServerOSInstalled_RequestedTemplateOnly(t *testing.T) {
	s := &Server{
		Status:           "failed",
		OSImageVersionID: 23,
		BuildFailed:      true,
	}
	if ServerOSInstalled(s) {
		t.Fatal("osTemplateInstallId alone must not count as installed OS")
	}
}

func TestServerReadyForGuestSetup_VFFailedWithoutOS(t *testing.T) {
	s := &Server{
		Status:           "failed",
		IP:               "66.248.206.61",
		OSImageVersionID: 0,
		BuildFailed:      true,
	}
	if ServerReadyForGuestSetup(s) {
		t.Fatal("failed without OS image must keep waiting or rebuild")
	}
}

func TestServerOSInstalled_VFFailedWithoutGuestAgent(t *testing.T) {
	s := &Server{
		Status:           "failed",
		OSImageVersionID: 7,
	}
	if ServerOSInstalled(s) {
		t.Fatal("failed shell with only template id must not count as OS installed")
	}
}

func TestServerNeedsBuild_VFFailedWithoutOS(t *testing.T) {
	s := &Server{
		Status:           "failed",
		IP:               "212.102.227.9",
		OSImageVersionID: 0,
	}
	if !ServerNeedsBuild(s) {
		t.Fatal("failed without OS image should trigger rebuild")
	}
}

func TestServerNeedsBuild_VFFailedWithOS(t *testing.T) {
	s := &Server{
		Status:           "failed",
		IP:               "212.102.227.9",
		OSImageVersionID: 7,
	}
	if ServerNeedsBuild(s) {
		t.Fatal("failed with OS image installed should not rebuild")
	}
}

func TestServerNeedsBuild_OpenStackActiveWithIP(t *testing.T) {
	s := &Server{
		Status:  "active",
		IP:      "203.0.113.10",
		OSBuilt: true,
	}
	if ServerNeedsBuild(s) {
		t.Fatal("active OpenStack instance with OS and IP should not rebuild")
	}
}

func TestServerNeedsBuild_OpenStackActiveMissingIP(t *testing.T) {
	s := &Server{
		Status:  "active",
		OSBuilt: true,
	}
	if !ServerNeedsBuild(s) {
		t.Fatal("active instance without public IP should finalize (floating ip)")
	}
}
