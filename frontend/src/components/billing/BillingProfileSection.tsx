import { useMemo, useState, type FormEvent } from 'react'
import { AlertTriangle, Building, Building2, CheckCircle2, FileText, Hash, Mail, MapPin, Phone, RefreshCw } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import type { BillingProfile } from '@/types'
import type { BillingProfileChange } from './types'
import { isBillingProfileComplete, validateBillingProfile } from './utils'
import { useBillingFormatters } from './useBillingFormatters'

type BillingProfileSectionProps = {
  profile: BillingProfile
  setProfile: BillingProfileChange
  savingProfile: boolean
  isProfileSaved: boolean
  handleSaveProfile: (event: FormEvent<HTMLFormElement>) => void | Promise<void>
}

export function BillingProfileSection({
  profile,
  setProfile,
  savingProfile,
  isProfileSaved,
  handleSaveProfile,
}: BillingProfileSectionProps) {
  const { t } = useBillingFormatters()
  const [touchedFields, setTouchedFields] = useState<Record<string, boolean>>({})
  const markTouched = (field: string) => setTouchedFields((current) => ({ ...current, [field]: true }))
  const profileValidation = useMemo(() => validateBillingProfile(profile), [profile])

  return (
      <Card id="billing-profile-card" className="border-border/60 bg-card shadow-xs overflow-hidden scroll-mt-6">
        <CardHeader className="border-b border-border/40 bg-muted/20 pb-5">
          <div>
            <CardTitle className="text-base font-bold tracking-tight flex items-center gap-2 text-foreground">
              <Building2 className="size-4.5 text-primary" />
              {t('billing.profile.title')}
            </CardTitle>
            <CardDescription className="mt-1 text-xs leading-relaxed text-muted-foreground">
              {t('billing.profile.description')}
            </CardDescription>
          </div>
        </CardHeader>
        <CardContent className="pt-6 space-y-6">
          {!isProfileSaved && !isBillingProfileComplete(profile) && (
            <div className="rounded-lg border border-amber-500/30 bg-amber-500/10 p-3.5 text-xs text-amber-600 dark:text-amber-400 font-medium flex items-start gap-2.5">
              <AlertTriangle className="size-4 shrink-0 text-amber-500 mt-0.5" />
              <div>
                <p className="font-semibold">{t('billing.profile.profileRequiredTitle')}</p>
                <p className="mt-0.5 text-muted-foreground">{t('billing.profile.incompleteBanner')}</p>
              </div>
            </div>
          )}

          <form onSubmit={handleSaveProfile} className="space-y-6">
            {/* Identity & Tax Identification Group */}
            <div className="space-y-4">
              <div className="border-b border-border/30 pb-2">
                <h4 className="text-xs font-bold uppercase tracking-wider text-muted-foreground flex items-center gap-1.5">
                  <Building2 className="size-3.5 text-primary" />
                  {t('billing.profile.identityTitle')}
                </h4>
              </div>

              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                {/* Full Name / Company Name */}
                <div className="space-y-1.5">
                  <Label htmlFor="company_name" className="text-xs font-semibold text-foreground flex items-center gap-1.5">
                    {t('billing.profile.companyName')}
                  </Label>
                  <div className="relative">
                    <Building2 className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 size-4 text-muted-foreground" />
                    <Input
                      id="company_name"
                      value={profile.company_name}
                      onBlur={() => markTouched('company_name')}
                      onChange={(e) => setProfile({ ...profile, company_name: e.target.value })}
                      placeholder={t('billing.profile.companyNamePlaceholder')}
                      className={`pl-9 text-xs h-9 ${
                        touchedFields.company_name && !profileValidation.isCompanyNameValid
                          ? 'border-destructive focus-visible:ring-destructive/20'
                          : ''
                      }`}
                    />
                  </div>
                  {touchedFields.company_name && !profileValidation.isCompanyNameValid ? (
                    <p className="text-[11px] text-destructive font-medium">{t('billing.profile.companyNameError')}</p>
                  ) : (
                    <p className="text-[11px] text-muted-foreground">{t('billing.profile.companyNameHint')}</p>
                  )}
                </div>

                {/* Tax ID / NPWP / NIK */}
                <div className="space-y-1.5">
                  <Label htmlFor="tax_id" className="text-xs font-semibold text-foreground flex items-center justify-between">
                    <span>{t('billing.profile.taxId')}</span>
                    <span className="text-[10px] font-normal text-muted-foreground">{t('billing.profile.optional')}</span>
                  </Label>
                  <div className="relative">
                    <FileText className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 size-4 text-muted-foreground" />
                    <Input
                      id="tax_id"
                      value={profile.tax_id}
                      onBlur={() => markTouched('tax_id')}
                      onChange={(e) => setProfile({ ...profile, tax_id: e.target.value })}
                      placeholder={t('billing.profile.taxIdPlaceholder')}
                      className={`pl-9 text-xs h-9 ${
                        touchedFields.tax_id && !profileValidation.isTaxIdValid
                          ? 'border-destructive focus-visible:ring-destructive/20'
                          : ''
                      }`}
                    />
                  </div>
                  {touchedFields.tax_id && !profileValidation.isTaxIdValid ? (
                    <p className="text-[11px] text-destructive font-medium">{t('billing.profile.taxIdError')}</p>
                  ) : (
                    <p className="text-[11px] text-muted-foreground">{t('billing.profile.taxIdHint')}</p>
                  )}
                </div>
              </div>
            </div>

            {/* Contact & Receipt Delivery Group */}
            <div className="space-y-4">
              <div className="border-b border-border/30 pb-2">
                <h4 className="text-xs font-bold uppercase tracking-wider text-muted-foreground flex items-center gap-1.5">
                  <Mail className="size-3.5 text-primary" />
                  {t('billing.profile.contactTitle')}
                </h4>
              </div>

              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                {/* Billing Email */}
                <div className="space-y-1.5">
                  <Label htmlFor="billing_email" className="text-xs font-semibold text-foreground">
                    {t('billing.profile.email')}
                  </Label>
                  <div className="relative">
                    <Mail className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 size-4 text-muted-foreground" />
                    <Input
                      id="billing_email"
                      type="email"
                      value={profile.email}
                      onBlur={() => markTouched('email')}
                      onChange={(e) => setProfile({ ...profile, email: e.target.value })}
                      placeholder={t('billing.profile.emailPlaceholder')}
                      className={`pl-9 text-xs h-9 ${
                        touchedFields.email && !profileValidation.isEmailValid
                          ? 'border-destructive focus-visible:ring-destructive/20'
                          : ''
                      }`}
                    />
                  </div>
                  {touchedFields.email && !profileValidation.isEmailValid ? (
                    <p className="text-[11px] text-destructive font-medium">{t('billing.profile.emailError')}</p>
                  ) : (
                    <p className="text-[11px] text-muted-foreground">{t('billing.profile.emailHint')}</p>
                  )}
                </div>

                {/* Phone Number */}
                <div className="space-y-1.5">
                  <Label htmlFor="billing_phone" className="text-xs font-semibold text-foreground">
                    {t('billing.profile.phone')}
                  </Label>
                  <div className="relative">
                    <Phone className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 size-4 text-muted-foreground" />
                    <Input
                      id="billing_phone"
                      value={profile.phone}
                      onBlur={() => markTouched('phone')}
                      onChange={(e) => setProfile({ ...profile, phone: e.target.value })}
                      placeholder={t('billing.profile.phonePlaceholder')}
                      className={`pl-9 text-xs h-9 ${
                        touchedFields.phone && !profileValidation.isPhoneValid
                          ? 'border-destructive focus-visible:ring-destructive/20'
                          : ''
                      }`}
                    />
                  </div>
                  {touchedFields.phone && !profileValidation.isPhoneValid ? (
                    <p className="text-[11px] text-destructive font-medium">{t('billing.profile.phoneError')}</p>
                  ) : (
                    <p className="text-[11px] text-muted-foreground">{t('billing.profile.phoneHint')}</p>
                  )}
                </div>
              </div>
            </div>

            {/* Address Information Group */}
            <div className="space-y-4">
              <div className="border-b border-border/30 pb-2">
                <h4 className="text-xs font-bold uppercase tracking-wider text-muted-foreground flex items-center gap-1.5">
                  <MapPin className="size-3.5 text-primary" />
                  {t('billing.profile.addressTitle')}
                </h4>
              </div>

              <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                {/* Street Address */}
                <div className="space-y-1.5 md:col-span-3">
                  <Label htmlFor="address_line1" className="text-xs font-semibold text-foreground">
                    {t('billing.profile.streetAddress')}
                  </Label>
                  <div className="relative">
                    <MapPin className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 size-4 text-muted-foreground" />
                    <Input
                      id="address_line1"
                      value={profile.address_line1}
                      onBlur={() => markTouched('address_line1')}
                      onChange={(e) => setProfile({ ...profile, address_line1: e.target.value })}
                      placeholder={t('billing.profile.streetAddressPlaceholder')}
                      className={`pl-9 text-xs h-9 ${
                        touchedFields.address_line1 && !profileValidation.isAddressValid
                          ? 'border-destructive focus-visible:ring-destructive/20'
                          : ''
                      }`}
                    />
                  </div>
                  {touchedFields.address_line1 && !profileValidation.isAddressValid && (
                    <p className="text-[11px] text-destructive font-medium">{t('billing.profile.streetAddressError')}</p>
                  )}
                </div>

                {/* City */}
                <div className="space-y-1.5">
                  <Label htmlFor="city" className="text-xs font-semibold text-foreground">
                    {t('billing.profile.city')}
                  </Label>
                  <div className="relative">
                    <Building className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 size-4 text-muted-foreground" />
                    <Input
                      id="city"
                      value={profile.city}
                      onBlur={() => markTouched('city')}
                      onChange={(e) => setProfile({ ...profile, city: e.target.value })}
                      placeholder={t('billing.profile.cityPlaceholder')}
                      className={`pl-9 text-xs h-9 ${
                        touchedFields.city && !profileValidation.isCityValid
                          ? 'border-destructive focus-visible:ring-destructive/20'
                          : ''
                      }`}
                    />
                  </div>
                  {touchedFields.city && !profileValidation.isCityValid && (
                    <p className="text-[11px] text-destructive font-medium">{t('billing.profile.cityError')}</p>
                  )}
                </div>

                {/* Postal Code */}
                <div className="space-y-1.5">
                  <Label htmlFor="postal_code" className="text-xs font-semibold text-foreground">
                    {t('billing.profile.postalCode')}
                  </Label>
                  <div className="relative">
                    <Hash className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 size-4 text-muted-foreground" />
                    <Input
                      id="postal_code"
                      value={profile.postal_code}
                      onBlur={() => markTouched('postal_code')}
                      onChange={(e) => setProfile({ ...profile, postal_code: e.target.value })}
                      placeholder={t('billing.profile.postalCodePlaceholder')}
                      className={`pl-9 text-xs h-9 ${
                        touchedFields.postal_code && !profileValidation.isPostalCodeValid
                          ? 'border-destructive focus-visible:ring-destructive/20'
                          : ''
                      }`}
                    />
                  </div>
                  {touchedFields.postal_code && !profileValidation.isPostalCodeValid && (
                    <p className="text-[11px] text-destructive font-medium">{t('billing.profile.postalCodeError')}</p>
                  )}
                </div>

                {/* Country Read-only Badge */}
                <div className="space-y-1.5">
                  <Label className="text-xs font-semibold text-foreground">
                    {t('billing.profile.country')}
                  </Label>
                  <div className="h-9 px-3 flex items-center rounded-md border border-border bg-muted/30 text-xs font-medium text-foreground">
                    {t('billing.profile.countryValue')}
                  </div>
                </div>
              </div>
            </div>

            <div className="flex flex-col sm:flex-row items-center justify-between gap-3 pt-4 border-t border-border/40">
              {!profileValidation.isValid ? (
                <p className="text-[11px] text-amber-600 dark:text-amber-400 font-medium flex items-center gap-1.5">
                  <AlertTriangle className="size-3.5 shrink-0" />
                  {t('billing.profile.fillAllFieldsHint')}
                </p>
              ) : (
                <span />
              )}
              <Button
                type="submit"
                size="sm"
                disabled={!profileValidation.isValid || savingProfile}
                className="font-semibold gap-1.5 hover:-translate-y-0.5 active:scale-[0.98] transition-all disabled:opacity-50 disabled:cursor-not-allowed disabled:hover:translate-y-0"
              >
                {savingProfile ? (
                  <>
                    <RefreshCw className="size-3.5 animate-spin" />
                    {t('billing.profile.saving')}
                  </>
                ) : (
                  <>
                    <CheckCircle2 className="size-3.5" />
                    {t('billing.profile.save')}
                  </>
                )}
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>
  )
}
