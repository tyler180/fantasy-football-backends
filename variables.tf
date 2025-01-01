variable "username" {
  type = string
}

variable "password" {
  type = string
}

variable "api_key" {
  type = string
}

variable "league_id" {
  type = string
}

variable "franchise_id" {
  type = string
}

variable "league_year" {
  type = string
  default = "2024"
}

variable "setjson" {
  type = string
  default = "1"
}

variable "setxml" {
  type = string
  default = "0"
}