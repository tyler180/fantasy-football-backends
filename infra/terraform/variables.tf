variable "username" {
  sensitive = true
  type      = string
}

variable "password" {
  sensitive = true
  type      = string
}

variable "api_key" {
  sensitive = true
  type      = string
}

variable "league_id" {
  type = string
}

variable "franchise_id" {
  type = string
}

variable "league_year" {
  type    = string
  default = "2025"
}

variable "setjson" {
  type    = string
  default = "1"
}

variable "setxml" {
  type    = string
  default = "0"
}

variable "season" {
  type    = string
  default = "2025"
}