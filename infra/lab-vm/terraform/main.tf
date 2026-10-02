locals {
  pool       = "default"
  image_path = abspath("${path.module}/../.cache/ubuntu-24.04-server-cloudimg-amd64.img")
  ssh_key    = trimspace(file(pathexpand(var.ssh_public_key_path)))
}

resource "libvirt_volume" "ubuntu_base" {
  name = "${var.vm_name}-ubuntu2404-base.qcow2"
  pool = local.pool
  target = {
    format = { type = "qcow2" }
  }
  create = {
    content = { url = local.image_path }
  }
}

resource "libvirt_volume" "system" {
  name       = "${var.vm_name}-system.qcow2"
  pool       = local.pool
  capacity   = var.disk_gib * 1024 * 1024 * 1024
  allocation = 0
  target = {
    format = { type = "qcow2" }
  }
  backing_store = {
    path   = libvirt_volume.ubuntu_base.path
    format = { type = "qcow2" }
  }
}

resource "libvirt_cloudinit_disk" "seed" {
  name = "${var.vm_name}-seed"
  user_data = templatefile("${path.module}/cloud-init.yaml.tftpl", {
    ssh_public_key = local.ssh_key
  })
  meta_data = yamlencode({
    "instance-id"    = var.vm_name
    "local-hostname" = var.vm_name
  })
}

resource "libvirt_volume" "seed" {
  name = "${var.vm_name}-seed.iso"
  pool = local.pool
  target = {
    format = { type = "raw" }
  }
  create = {
    content = { url = libvirt_cloudinit_disk.seed.path }
  }
}

resource "libvirt_domain" "lab" {
  name        = var.vm_name
  description = "Dedicated Ubuntu 24.04 DevStack single-node thesis laboratory"
  type        = "kvm"
  running     = true
  autostart   = false
  memory      = var.memory_mib
  memory_unit = "MiB"
  vcpu        = var.vcpus

  os = {
    type         = "hvm"
    type_arch    = "x86_64"
    type_machine = "q35"
  }

  devices = {
    disks = [
      {
        device = "disk"
        source = { file = { file = libvirt_volume.system.path } }
        target = { dev = "vda", bus = "virtio" }
      },
      {
        device    = "cdrom"
        read_only = true
        source    = { file = { file = libvirt_volume.seed.path } }
        target    = { dev = "sda", bus = "sata" }
      }
    ]
    interfaces = [
      {
        model  = { type = "virtio" }
        mac    = { address = "52:54:00:24:10:02" }
        source = { network = { network = "default" } }
        wait_for_ip = {
          network = "192.168.122.0/24"
          source  = "lease"
          timeout = 300
        }
      }
    ]
  }
}
