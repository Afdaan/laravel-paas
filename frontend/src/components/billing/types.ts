import type { Dispatch, SetStateAction } from 'react'

import type { BillingProfile, BillingStatus, TopupPackage } from '@/types'

export type AttentionResource = BillingStatus & {
  resource_name?: string
}

export type PendingRenewChange = {
  resource_id: number
  resource_type: 'project' | 'database'
  resource_name: string
  target_auto_renew: boolean
}

export type PendingTopup = {
  type: 'package' | 'custom'
  package?: TopupPackage
  customAmount?: number
  credits: number
  amountMinor: number
  currency: string
}

export type BillingProfileValidation = {
  isValid: boolean
  isCompanyNameValid: boolean
  isEmailValid: boolean
  isPhoneValid: boolean
  isAddressValid: boolean
  isCityValid: boolean
  isPostalCodeValid: boolean
  isTaxIdValid: boolean
}

export type BillingProfileChange = Dispatch<SetStateAction<BillingProfile>>
