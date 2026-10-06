'use client';
import Link from 'next/link';
import { useEffect, useState } from 'react';
import { api, WeatherHour } from '@/lib/api';
import { latestWeather, validateWeatherHistory, weatherTimezone } from '@/lib/weather';
import { loggedDayKey, loggedDayLabel, loggedDayTime } from '@/lib/loggedDay';
import { dateLocaleFor, interpolate, numberLocaleFor } from '@/lib/i18n';
import { useLanguage } from './LanguageContext';
import TapTarget from './ui/TapTarget';
import { EyeIcon, EyeOffIcon } from './icons';

interface Props {
  timezone?: string; editing?: boolean; hidden?: boolean; controlsDisabled?: boolean;
  onMoveUp?: () => void; onMoveDown?: () => void; moveUpDisabled?: boolean; moveDownDisabled?: boolean; onToggleHidden?: () => void;
}
export default function WeatherCard(props: Props) {
  const {t, language} = useLanguage();
  const [state, setState] = useState<{kind:'loading'|'error'|'empty'|'pending'|'ready'; hour?:WeatherHour}>({kind:'loading'});
  const zone = weatherTimezone(props.timezone);
  useEffect(() => {
    let cancelled = false;
    setState({kind:'loading'});
    const now = new Date();
    api.weatherHistory(new Date(now.getTime() - 7 * 86400000).toISOString(), now.toISOString())
      .then(validateWeatherHistory).then(history => {
        if (cancelled) return;
        const hour = latestWeather(history,now);
        setState(hour ? {kind:'ready',hour} : {kind:history.gaps.some(g=>g.reason==='pending_weather')?'pending':'empty'});
      }).catch(()=>{if(!cancelled)setState({kind:'error'});});
    return ()=>{cancelled=true;};
  }, [zone]);
  const hour=state.hour;
  const date=loggedDayKey(hour?new Date(hour.hour):new Date(),zone);
  const href=`/weather/day/?${new URLSearchParams({date, timezone:zone})}`;
  const label=t('weather.title');
  const dim=props.editing&&props.hidden?' opacity-40':'';
  const content=<>
    <p className={`font-[family-name:var(--font-data)] text-[11px] font-bold uppercase tracking-wide mb-2${dim}`}>{label}</p>
    {props.editing&&<div className="flex flex-wrap justify-end gap-1.5 mb-2">
      <TapTarget onClick={props.onMoveUp} disabled={props.moveUpDisabled||props.controlsDisabled} aria-label={interpolate(t('vitals.moveUp'),{metric:label})} className="rounded-md border border-border bg-bg text-text disabled:opacity-30">↑</TapTarget>
      <TapTarget onClick={props.onMoveDown} disabled={props.moveDownDisabled||props.controlsDisabled} aria-label={interpolate(t('vitals.moveDown'),{metric:label})} className="rounded-md border border-border bg-bg text-text disabled:opacity-30">↓</TapTarget>
      <TapTarget onClick={props.onToggleHidden} disabled={props.controlsDisabled} data-testid="weather-card-visibility" aria-label={interpolate(t(props.hidden?'dashboard.showCard':'dashboard.hideCard'),{metric:label})} className="rounded-md border border-border bg-bg text-text disabled:opacity-30">{props.hidden?<EyeOffIcon className="w-4 h-4"/>:<EyeIcon className="w-4 h-4"/>}</TapTarget>
    </div>}
    <div className={dim}>
    {hour?<>
      <p className="font-[family-name:var(--font-data)] text-xl font-bold tabular-nums">{hour.temperature_c.toLocaleString(numberLocaleFor(language),{maximumFractionDigits:1})} °C</p>
      <p className="text-xs tabular-nums">{hour.surface_pressure_hpa.toLocaleString(numberLocaleFor(language),{maximumFractionDigits:1})} hPa</p>
      <p className="text-[11px] text-text-muted mt-1" data-testid="weather-card-as-of">{interpolate(t('weather.saved'),{date:`${loggedDayLabel(new Date(hour.hour),dateLocaleFor(language),zone)} ${loggedDayTime(new Date(hour.hour),dateLocaleFor(language),zone)}`})}</p>
    </>:<p className="text-xs text-text-muted">{t(state.kind==='loading'?'weather.loading':state.kind==='error'?'weather.error':state.kind==='pending'?'weather.pending':'weather.empty')}</p>}
    </div>
  </>;
  const className='block bg-bg-elevated border border-border rounded-[10px] px-3.5 py-3 hover:border-accent transition-colors';
  return props.editing?<div className={className} data-testid="weather-card" data-hidden={props.hidden?'true':'false'}>{content}</div>:<Link className={className} href={href} data-testid="weather-card" aria-label={t('weather.details')}>{content}</Link>;
}
