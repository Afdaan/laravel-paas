import type { BillingOverview, BillingProfile } from '@/types'
import type { BillingProfileValidation } from './types'

export function getInvoiceNumber(invoice: Pick<BillingOverview['invoices'][number], 'id' | 'period_start' | 'created_at'> & { invoice_number?: string }) {
  if (invoice.invoice_number && invoice.invoice_number.trim() !== '') {
    return invoice.invoice_number
  }
  const date = new Date(invoice.period_start || invoice.created_at || Date.now())
  const year = date.getFullYear() || 2026
  const month = String(date.getMonth() + 1).padStart(2, '0')
  return `INV-${year}${month}-${String(invoice.id).padStart(4, '0')}`
}

export function isValidPhoneNumber(phone: string, country: string = 'ID'): boolean {
  const trimmed = phone.trim()
  if (!trimmed) return false

  const digitsOnly = trimmed.replace(/\D/g, '')
  if (country === 'ID' || country === 'IDN') {
    if (digitsOnly.startsWith('08')) return digitsOnly.length >= 10 && digitsOnly.length <= 13
    if (digitsOnly.startsWith('628')) return digitsOnly.length >= 11 && digitsOnly.length <= 14
    if (digitsOnly.startsWith('8')) return digitsOnly.length >= 9 && digitsOnly.length <= 12
    return false
  }

  return digitsOnly.length >= 7 && digitsOnly.length <= 15
}

export function validateBillingProfile(profile: BillingProfile): BillingProfileValidation {
  const companyName = profile.company_name.trim()
  const email = profile.email.trim()
  const address = profile.address_line1.trim()
  const city = profile.city.trim()
  const postalCode = profile.postal_code.trim()
  const taxID = profile.tax_id.replace(/\D/g, '')
  const isCompanyNameValid = companyName.length >= 2
  const isEmailValid = /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)
  const isPhoneValid = isValidPhoneNumber(profile.phone, profile.country)
  const isAddressValid = address.length >= 5
  const isCityValid = city.length >= 2
  const isPostalCodeValid = /^[0-9]{5}$/.test(postalCode)
  const isTaxIdValid = !profile.tax_id.trim() || taxID.length >= 15

  return {
    isValid:
      isCompanyNameValid &&
      isEmailValid &&
      isPhoneValid &&
      isAddressValid &&
      isCityValid &&
      isPostalCodeValid &&
      isTaxIdValid,
    isCompanyNameValid,
    isEmailValid,
    isPhoneValid,
    isAddressValid,
    isCityValid,
    isPostalCodeValid,
    isTaxIdValid,
  }
}

export function isBillingProfileComplete(profile: BillingProfile) {
  const validation = validateBillingProfile(profile)
  return (
    validation.isCompanyNameValid &&
    validation.isEmailValid &&
    validation.isPhoneValid &&
    validation.isAddressValid &&
    validation.isCityValid &&
    validation.isPostalCodeValid
  )
}
