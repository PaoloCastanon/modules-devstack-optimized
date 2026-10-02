output "vm_name" {
  value = libvirt_domain.lab.name
}

output "disk_path" {
  value = libvirt_volume.system.path
}
