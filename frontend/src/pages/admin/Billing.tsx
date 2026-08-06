import { useCallback, useEffect, useState } from 'react'
import { AlertTriangle, CreditCard, ReceiptText, RefreshCw, WalletCards } from 'lucide-react'
import { toast } from 'sonner'
import { billingAPI } from '@/services/api'
import useAuthStore from '@/stores/authStore'
import useTranslation from '@/lib/useTranslation'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'

type Catalog = {
  specs: Array<{ id: number; name: string; slug: string; type: 'project' | 'database'; version: number; monthly_credits: number; cpu_millicores: number; memory_mb: number; storage_gb: number; connection_limit?: number; backup_retention_days?: number; is_active: boolean }>
  packages: Array<{ id: number; credits: number; amount_minor: number; currency: string; version: number; is_active: boolean }>
}
type Suspension = { user_id: number; resource_id: number; resource_type: string; status: string; oldest_due_at?: string; payment_due_days: number }
type Page<T> = { data: T[]; page: number; limit: number; total: number }
type Wallet = { user_id: number; balance_credits: number; updated_at: string }
type Invoice = { id: number; user_id: number; total_credits: number; status: string; period_start: string; period_end: string; due_at?: string; paid_at?: string }
type Topup = { id: number; user_id: number; credits: number; amount_minor: number; currency: string; status: string; created_at: string; paid_at?: string }

const PAGE_LIMIT = 20
const emptySpec = { type: 'project', name: '', slug: '', cpu_millicores: 500, memory_mb: 512, storage_gb: 1, monthly_credits: 0, connection_limit: '', backup_retention_days: '', reason: '' }
const emptyPackage = { credits: 0, amount_minor: 0, sort_order: 0, reason: '' }
const formatCredits = (value: number) => new Intl.NumberFormat().format(value)
const statusVariant = (status: string) => status === 'paid' || status === 'active' ? 'secondary' : status === 'suspended' || status === 'payment_due' ? 'destructive' : 'outline'

export default function AdminBilling() {
  const { t, language } = useTranslation()
  const isSuperAdmin = useAuthStore((state) => state.user?.role === 'superadmin')
  const [catalog, setCatalog] = useState<Catalog | null>(null)
  const [wallets, setWallets] = useState<Page<Wallet> | null>(null)
  const [invoices, setInvoices] = useState<Page<Invoice> | null>(null)
  const [topups, setTopups] = useState<Page<Topup> | null>(null)
  const [suspensions, setSuspensions] = useState<Suspension[] | null>(null)
  const [walletPage, setWalletPage] = useState(1)
  const [invoicePage, setInvoicePage] = useState(1)
  const [topupPage, setTopupPage] = useState(1)
  const [errors, setErrors] = useState<string[]>([])
  const [loading, setLoading] = useState(true)
  const [specForm, setSpecForm] = useState(emptySpec)
  const [packageForm, setPackageForm] = useState(emptyPackage)
  const [creatingSpec, setCreatingSpec] = useState(false)
  const [creatingPackage, setCreatingPackage] = useState(false)

  const formatDate = useCallback((value?: string) => value ? new Intl.DateTimeFormat(language === 'id' ? 'id-ID' : 'en-US', { dateStyle: 'medium' }).format(new Date(value)) : '—', [language])
  const formatMoney = useCallback((amountMinor: number, currency: string) => new Intl.NumberFormat(language === 'id' ? 'id-ID' : 'en-US', { style: 'currency', currency, maximumFractionDigits: 0 }).format(amountMinor), [language])
  const formatStatus = useCallback((status: string) => {
    const translated = t(`billing.statuses.${status}`)
    return translated === `billing.statuses.${status}` ? status.replaceAll('_', ' ') : translated
  }, [t])

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const results = await Promise.allSettled([
        billingAPI.adminCatalog(),
        billingAPI.adminWallets({ page: walletPage, limit: PAGE_LIMIT }),
        billingAPI.adminInvoices({ page: invoicePage, limit: PAGE_LIMIT }),
        billingAPI.adminTopups({ page: topupPage, limit: PAGE_LIMIT }),
        billingAPI.adminSuspensions(),
      ])
      const nextErrors: string[] = []
      if (results[0].status === 'fulfilled') setCatalog(results[0].value.data); else nextErrors.push(t('billing.plans'))
      if (results[1].status === 'fulfilled') setWallets(results[1].value.data); else nextErrors.push(t('billing.admin.wallets'))
      if (results[2].status === 'fulfilled') setInvoices(results[2].value.data); else nextErrors.push(t('billing.invoices'))
      if (results[3].status === 'fulfilled') setTopups(results[3].value.data); else nextErrors.push(t('billing.topups'))
      if (results[4].status === 'fulfilled') setSuspensions(results[4].value.data); else nextErrors.push(t('billing.admin.suspensions'))
      setErrors(nextErrors)
    } finally {
      setLoading(false)
    }
  }, [invoicePage, t, topupPage, walletPage])

  useEffect(() => {
    void load()
  }, [load])

  const createSpec = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (!isSuperAdmin) return
    setCreatingSpec(true)
    try {
      await billingAPI.createSpec({
        ...specForm,
        connection_limit: specForm.connection_limit === '' ? undefined : Number(specForm.connection_limit),
        backup_retention_days: specForm.backup_retention_days === '' ? undefined : Number(specForm.backup_retention_days),
      })
      setSpecForm(emptySpec)
      toast.success(t('billing.admin.createSucceeded'))
      await load()
    } catch {
      toast.error(t('billing.admin.createFailed'))
    } finally {
      setCreatingSpec(false)
    }
  }

  const createPackage = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (!isSuperAdmin) return
    setCreatingPackage(true)
    try {
      await billingAPI.createTopupPackage(packageForm)
      setPackageForm(emptyPackage)
      toast.success(t('billing.admin.createSucceeded'))
      await load()
    } catch {
      toast.error(t('billing.admin.createFailed'))
    } finally {
      setCreatingPackage(false)
    }
  }

  return <div className="mx-auto max-w-7xl space-y-6 pb-10">
    <div className="flex flex-col gap-3 border-b pb-5 sm:flex-row sm:items-end sm:justify-between">
      <div><p className="text-sm font-medium text-primary">{t('common.adminPanel')}</p><h1 className="text-2xl font-semibold tracking-tight">{t('billing.admin.title')}</h1><p className="mt-1 text-sm text-muted-foreground">{t('billing.admin.description')}</p></div>
      <Button variant="outline" size="sm" onClick={() => void load()} disabled={loading}><RefreshCw className={loading ? 'animate-spin' : ''} /> {t('billing.refresh')}</Button>
    </div>

    {errors.length > 0 && <Card className="border-amber-500/40 bg-amber-500/5" role="status"><CardContent className="pt-6 text-sm">{t('billing.loadPartial', { sections: errors.join(', ') })}</CardContent></Card>}

    <div className="grid gap-6 xl:grid-cols-2">
      <CatalogCard title={t('billing.admin.resourcePlans')} rows={catalog?.specs.map((spec) => ({ id: spec.id, title: `${spec.name} · ${formatCredits(spec.monthly_credits)} ${t('billing.credits')}`, detail: `${spec.type} · v${spec.version} · ${spec.cpu_millicores}m CPU · ${spec.memory_mb} MB${spec.connection_limit ? ` · ${t('billing.connections', { count: spec.connection_limit })}` : ''}`, active: spec.is_active }))} loading={loading} empty={t('billing.admin.noRecords')} />
      <CatalogCard title={t('billing.admin.topupPackages')} rows={catalog?.packages.map((pkg) => ({ id: pkg.id, title: `${formatCredits(pkg.credits)} ${t('billing.credits')} · ${formatMoney(pkg.amount_minor, pkg.currency)}`, detail: `v${pkg.version}`, active: pkg.is_active }))} loading={loading} empty={t('billing.admin.noRecords')} />
    </div>

    {isSuperAdmin ? <section className="grid gap-6 xl:grid-cols-2">
      <PricingForm title={t('billing.admin.createPlan')} description={t('billing.admin.versionedPricing')} onSubmit={createSpec} submitting={creatingSpec} submitLabel={t('billing.admin.create')}>
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label={t('billing.admin.resourceType')} htmlFor="spec-type"><Select value={specForm.type} onValueChange={(type) => setSpecForm((current) => ({ ...current, type }))}><SelectTrigger id="spec-type" className="w-full">{specForm.type === 'project' ? t('billing.admin.project') : t('billing.admin.database')}</SelectTrigger><SelectContent><SelectItem value="project">{t('billing.admin.project')}</SelectItem><SelectItem value="database">{t('billing.admin.database')}</SelectItem></SelectContent></Select></Field>
          <NumberField id="spec-monthly-credits" label={t('billing.admin.monthlyCredits')} value={specForm.monthly_credits} onChange={(monthly_credits) => setSpecForm((current) => ({ ...current, monthly_credits }))} min={1} />
          <TextField id="spec-name" label={t('billing.admin.name')} value={specForm.name} onChange={(name) => setSpecForm((current) => ({ ...current, name }))} required />
          <TextField id="spec-slug" label={t('billing.admin.slug')} value={specForm.slug} onChange={(slug) => setSpecForm((current) => ({ ...current, slug }))} pattern="[a-z0-9-]+" required />
          <NumberField id="spec-cpu" label={t('billing.admin.cpu')} value={specForm.cpu_millicores} onChange={(cpu_millicores) => setSpecForm((current) => ({ ...current, cpu_millicores }))} min={1} />
          <NumberField id="spec-memory" label={t('billing.admin.memory')} value={specForm.memory_mb} onChange={(memory_mb) => setSpecForm((current) => ({ ...current, memory_mb }))} min={1} />
          <NumberField id="spec-storage" label={t('billing.admin.storage')} value={specForm.storage_gb} onChange={(storage_gb) => setSpecForm((current) => ({ ...current, storage_gb }))} min={1} />
          <Field label={t('billing.admin.connectionLimit')} htmlFor="spec-connection-limit"><Input id="spec-connection-limit" type="number" min={1} value={specForm.connection_limit} onChange={(event) => setSpecForm((current) => ({ ...current, connection_limit: event.target.value === '' ? '' : Number(event.target.value) }))} /></Field>
          <Field label={t('billing.admin.backupRetentionDays')} htmlFor="spec-backup-retention"><Input id="spec-backup-retention" type="number" min={1} value={specForm.backup_retention_days} onChange={(event) => setSpecForm((current) => ({ ...current, backup_retention_days: event.target.value === '' ? '' : Number(event.target.value) }))} /></Field>
        </div>
        <Field label={t('billing.admin.reason')} htmlFor="spec-reason"><Textarea id="spec-reason" value={specForm.reason} onChange={(event) => setSpecForm((current) => ({ ...current, reason: event.target.value }))} required /></Field>
      </PricingForm>
      <PricingForm title={t('billing.admin.createPackage')} description={t('billing.admin.versionedPricing')} onSubmit={createPackage} submitting={creatingPackage} submitLabel={t('billing.admin.create')}>
        <div className="grid gap-3 sm:grid-cols-2"><NumberField label={t('billing.admin.credits')} value={packageForm.credits} onChange={(credits) => setPackageForm((current) => ({ ...current, credits }))} min={1} /><NumberField label={t('billing.admin.amountMinor')} value={packageForm.amount_minor} onChange={(amount_minor) => setPackageForm((current) => ({ ...current, amount_minor }))} min={1} /><NumberField label={t('billing.admin.sortOrder')} value={packageForm.sort_order} onChange={(sort_order) => setPackageForm((current) => ({ ...current, sort_order }))} min={0} /></div>
        <Field label={t('billing.admin.reason')}><Textarea value={packageForm.reason} onChange={(event) => setPackageForm((current) => ({ ...current, reason: event.target.value }))} required /></Field>
      </PricingForm>
    </section> : <Card><CardContent className="pt-6 text-sm text-muted-foreground">{t('billing.admin.superadminOnly')}</CardContent></Card>}

    <section className="grid gap-6 xl:grid-cols-3">
      <CollectionCard title={t('billing.admin.wallets')} icon={<WalletCards className="size-4" />} page={wallets} loading={loading} empty={t('billing.admin.noRecords')} meta={(wallet) => `${t('billing.admin.user')} #${wallet.user_id} · ${formatDate(wallet.updated_at)}`} value={(wallet) => `${formatCredits(wallet.balance_credits)} ${t('billing.credits')}`} onPageChange={setWalletPage} />
      <CollectionCard title={t('billing.invoices')} icon={<ReceiptText className="size-4" />} page={invoices} loading={loading} empty={t('billing.admin.noRecords')} meta={(invoice) => `${t('billing.admin.user')} #${invoice.user_id} · ${formatDate(invoice.period_start)} – ${formatDate(invoice.period_end)}`} value={(invoice) => `${formatCredits(invoice.total_credits)} ${t('billing.credits')}`} status={(invoice) => invoice.status} formatStatus={formatStatus} onPageChange={setInvoicePage} />
      <CollectionCard title={t('billing.topups')} icon={<CreditCard className="size-4" />} page={topups} loading={loading} empty={t('billing.admin.noRecords')} meta={(topup) => `${t('billing.admin.user')} #${topup.user_id} · ${formatDate(topup.paid_at ?? topup.created_at)}`} value={(topup) => `${formatCredits(topup.credits)} ${t('billing.credits')} · ${formatMoney(topup.amount_minor, topup.currency)}`} status={(topup) => topup.status} formatStatus={formatStatus} onPageChange={setTopupPage} />
    </section>

    <Card><CardHeader><CardTitle className="flex items-center gap-2 text-base"><AlertTriangle className="size-4 text-destructive" />{t('billing.admin.suspensions')}</CardTitle></CardHeader><CardContent className="space-y-3">{suspensions?.map((item) => <div key={`${item.user_id}-${item.resource_type}-${item.resource_id}`} className="flex items-center justify-between gap-3 border-b pb-3 last:border-0"><div><p className="text-sm font-medium">{t('billing.admin.user')} #{item.user_id} · {item.resource_type} #{item.resource_id}</p><p className="text-xs text-muted-foreground">{item.oldest_due_at ? `${formatDate(item.oldest_due_at)} · ` : ''}{t('billing.days', { count: item.payment_due_days })}</p></div><Badge variant={statusVariant(item.status)}>{formatStatus(item.status)}</Badge></div>)}{suspensions !== null && suspensions.length === 0 && <p className="text-sm text-muted-foreground">{t('billing.admin.noRecords')}</p>}</CardContent></Card>
  </div>
}

function CatalogCard({ title, rows, loading, empty }: { title: string; rows?: Array<{ id: number; title: string; detail: string; active: boolean }>; loading: boolean; empty: string }) {
  const { t } = useTranslation()
  return <Card><CardHeader><CardTitle className="text-base">{title}</CardTitle></CardHeader><CardContent className="space-y-3">{loading && Array.from({ length: 3 }).map((_, i) => <Skeleton key={i} className="h-14 w-full" />)}{!loading && rows?.map((row) => <div key={row.id} className="flex items-center justify-between gap-3 border-b pb-3 last:border-0"><div><p className="text-sm font-medium">{row.title}</p><p className="text-xs text-muted-foreground">{row.detail}</p></div><Badge variant={row.active ? 'secondary' : 'outline'}>{row.active ? t('billing.admin.active') : t('billing.admin.inactive')}</Badge></div>)}{!loading && rows?.length === 0 && <p className="text-sm text-muted-foreground">{empty}</p>}</CardContent></Card>
}

function CollectionCard<T extends { id?: number; user_id: number }>({ title, icon, page, loading, empty, meta, value, status, formatStatus = (status) => status.replace('_', ' '), onPageChange }: { title: string; icon: React.ReactNode; page: Page<T> | null; loading: boolean; empty: string; meta: (row: T) => string; value: (row: T) => string; status?: (row: T) => string; formatStatus?: (status: string) => string; onPageChange: (page: number) => void }) {
  const { t } = useTranslation()
  const hasPrevious = (page?.page ?? 1) > 1
  const hasNext = page ? page.page * page.limit < page.total : false
  return <Card><CardHeader><CardTitle className="flex items-center gap-2 text-base">{icon}{title}</CardTitle><CardDescription>{page ? t('billing.admin.total', { count: page.data.length, total: page.total }) : ''}</CardDescription></CardHeader><CardContent className="space-y-3">{loading && Array.from({ length: 3 }).map((_, i) => <Skeleton key={i} className="h-14 w-full" />)}{!loading && page?.data.map((row, index) => <div key={row.id ?? `${row.user_id}-${index}`} className="flex items-center justify-between gap-3 border-b pb-3 last:border-0"><div className="min-w-0"><p className="text-sm font-medium">{value(row)}</p><p className="truncate text-xs text-muted-foreground">{meta(row)}</p></div>{status && <Badge variant={statusVariant(status(row))}>{formatStatus(status(row))}</Badge>}</div>)}{!loading && page?.data.length === 0 && <p className="text-sm text-muted-foreground">{empty}</p>}{page && <div className="flex justify-end gap-2 pt-1"><Button size="sm" variant="outline" disabled={!hasPrevious || loading} onClick={() => onPageChange(page.page - 1)}>{t('common.previous')}</Button><Button size="sm" variant="outline" disabled={!hasNext || loading} onClick={() => onPageChange(page.page + 1)}>{t('common.next')}</Button></div>}</CardContent></Card>
}

function PricingForm({ title, description, children, onSubmit, submitting, submitLabel }: { title: string; description: string; children: React.ReactNode; onSubmit: (event: React.FormEvent<HTMLFormElement>) => void; submitting: boolean; submitLabel: string }) {
  return <Card><CardHeader><CardTitle className="text-base">{title}</CardTitle><CardDescription>{description}</CardDescription></CardHeader><CardContent><form className="space-y-4" onSubmit={onSubmit}>{children}<Button type="submit" disabled={submitting}>{submitting ? '…' : submitLabel}</Button></form></CardContent></Card>
}

function Field({ label, htmlFor, children }: { label: string; htmlFor?: string; children: React.ReactNode }) { return <div className="space-y-1.5"><Label htmlFor={htmlFor}>{label}</Label>{children}</div> }
function TextField({ label, id, value, onChange, required, pattern }: { label: string; id?: string; value: string; onChange: (value: string) => void; required?: boolean; pattern?: string }) { return <Field label={label} htmlFor={id}><Input id={id} value={value} onChange={(event) => onChange(event.target.value)} required={required} pattern={pattern} /></Field> }
function NumberField({ label, id, value, onChange, min }: { label: string; id?: string; value: number; onChange: (value: number) => void; min: number }) { return <Field label={label} htmlFor={id}><Input id={id} type="number" min={min} value={value} onChange={(event) => onChange(Number(event.target.value))} required /></Field> }
