output "vm_name" {
  value = libvirt_domain.lab.name
}

output "management_ip" {
  value = local.management_ip
}

output "disk_path" {
  value = libvirt_volume.system.path
}
