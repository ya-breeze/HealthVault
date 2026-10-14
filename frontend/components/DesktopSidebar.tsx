'use client';
import Link from 'next/link';
import { usePathname } from 'next/navigation';
import type { Me } from '@/lib/api';
import { useLanguage } from './LanguageContext';
import { HomeIcon, HistoryIcon, CameraIcon, PencilIcon, ImportIcon, SettingsIcon } from './icons';
import { isSameRoute } from './nav';

export default function DesktopSidebar({me}: {me: Me}) {
  const pathname = usePathname();
  const {t} = useLanguage();
  const destinations = [
    {href: '/', label: t('nav.home'), Icon: HomeIcon},
    {href: '/food/history/', label: t('dashboard.history'), Icon: HistoryIcon},
    {href: '/food/upload/', label: t('dashboard.photo'), Icon: CameraIcon},
    {href: '/food/manual/', label: t('dashboard.manual'), Icon: PencilIcon},
    {href: '/food/custom/', label: t('header.customFoods'), Icon: HistoryIcon},
    {href: '/import/', label: t('header.import'), Icon: ImportIcon},
    {href: '/settings/', label: t('header.settings'), Icon: SettingsIcon},
  ];
  return <aside className="hv-sidebar" data-testid="desktop-sidebar">
    <Link href="/" className="hv-sidebar-brand"><span className="hv-brand-mark" aria-hidden="true"><span>h<sup>+</sup></span></span>HealthVault</Link>
    <p className="hv-sidebar-caption">{t('sidebar.caption')}</p>
    <nav aria-label={t('nav.label')}>{destinations.map(({href,label,Icon}) => <Link key={href} href={href} aria-current={isSameRoute(pathname,href)?'page':undefined}><Icon className="w-5 h-5 shrink-0" aria-hidden="true"/>{label}</Link>)}</nav>
    <Link href="/settings/" className="hv-sidebar-account"><span className="hv-avatar" aria-hidden="true">{me.username.slice(0,1).toUpperCase()}</span><span>{me.username}</span></Link>
  </aside>;
}
