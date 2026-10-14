'use client';
import { useEffect, useState } from 'react';
import Link from 'next/link';
import { useRouter, useSearchParams } from 'next/navigation';
import { CartesianGrid, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts';
import { api, WeatherHistory } from '@/lib/api';
import { dayWeather, validWeatherDate, validateWeatherHistory, weatherQueryBounds, weatherTimezone } from '@/lib/weather';
import { loggedDayKey } from '@/lib/loggedDay';
import { addDays } from '@/lib/foodLogHistory';
import { dateLocaleFor, Dictionary, interpolate, numberLocaleFor } from '@/lib/i18n';
import { replayTouchAsMove } from '@/lib/chartTouch';
import useCoarsePointer from '@/lib/useCoarsePointer';
import AuthenticatedShell from '@/components/AuthenticatedShell';
import { useLanguage } from '@/components/LanguageContext';
import TapTarget from '@/components/ui/TapTarget';

const gapKeys:Record<string,keyof Dictionary>={missing_location:'weather.missing_location',moving:'weather.moving',poor_accuracy:'weather.poor_accuracy',long_gap:'weather.long_gap',pending_weather:'weather.pending_weather',incomplete_hour:'weather.incomplete_hour'};
export default function WeatherDayClient() {
  const {t,language}=useLanguage();const params=useSearchParams();const router=useRouter();const coarse=useCoarsePointer();
  const [accountZone,setAccountZone]=useState<string|null>(null);const [settingsError,setSettingsError]=useState(false);
  const [history,setHistory]=useState<WeatherHistory|null>(null);const [error,setError]=useState(false);const [loading,setLoading]=useState(true);const [retry,setRetry]=useState(0);
  const override=params.get('timezone');const zone=weatherTimezone(override||accountZone);const rawDate=params.get('date');
  const ready=accountZone!==null||!!override;
  const date=rawDate||loggedDayKey(new Date(),zone);const valid=validWeatherDate(date);const today=loggedDayKey(new Date(),zone);
  useEffect(()=>{let cancelled=false;setSettingsError(false);api.getSettings().then(s=>{if(!cancelled)setAccountZone(weatherTimezone(s.timezone));}).catch(()=>{if(!cancelled)setSettingsError(true);});return()=>{cancelled=true;};},[retry]);
  useEffect(()=>{
    let cancelled=false;setHistory(null);setError(false);setLoading(true);
    if(!ready)return()=>{cancelled=true;};
    if(!valid){setLoading(false);return()=>{cancelled=true;};}
    try {
      const range=weatherQueryBounds(date,zone,new Date());
      if(!range.elapsed){setHistory({hours:[],gaps:[],units:{}});setLoading(false);return()=>{cancelled=true;};}
      api.weatherHistory(range.from,range.to).then(validateWeatherHistory).then(h=>{if(!cancelled){setHistory(h);setLoading(false);}}).catch(()=>{if(!cancelled){setError(true);setLoading(false);}});
    } catch {setError(true);setLoading(false);}
    return()=>{cancelled=true;};
  },[date,zone,valid,ready,retry]);
  function go(next:string){const q=new URLSearchParams({date:next});if(override)q.set('timezone',override);router.push(`/weather/day/?${q}`);}
  const range=valid?(()=>{try{return weatherQueryBounds(date,zone,new Date());}catch{return null;}})():null;
  const data=history&&range?dayWeather(history,range.day.from,range.day.to):null;
  const time=(ms:number)=>new Date(ms).toLocaleTimeString(dateLocaleFor(language),{timeZone:zone,hour:'2-digit',minute:'2-digit',timeZoneName:'shortOffset'});
  const number=(v:number)=>v.toLocaleString(numberLocaleFor(language),{maximumFractionDigits:1});
  const failed=error||(settingsError&&!override);
  return <AuthenticatedShell><main className="hv-page" data-testid="weather-day">
    <div className="hv-page-heading"><h1 className="text-text">{t('weather.title')}</h1><TapTarget as={Link} href="/" className="text-accent text-sm flex items-center">{t('weather.back')}</TapTarget></div>
    <p className="text-sm text-text-muted mb-6">{interpolate(t(override?'weather.linkZone':'weather.zone'),{zone})}</p>
    <div className="flex flex-wrap items-center gap-3 mb-6">
      <TapTarget disabled={!valid} onClick={()=>go(addDays(date,-1))} aria-label={t('weather.previous')} className="border border-border rounded-lg px-3 disabled:opacity-30">←</TapTarget>
      <label className="text-sm">{t('weather.date')} <input data-testid="weather-date" type="date" value={valid?date:''} max={today} onChange={e=>go(e.target.value)} className="bg-bg-elevated border border-border rounded-lg p-2 text-text min-h-12"/></label>
      <TapTarget disabled={!valid||date>=today} onClick={()=>go(addDays(date,1))} aria-label={t('weather.next')} className="border border-border rounded-lg px-3 disabled:opacity-30">→</TapTarget>
    </div>
    {!valid?<p role="alert">{t('weather.invalidDate')}</p>:failed?<div role="alert"><p>{t('weather.error')}</p><TapTarget onClick={()=>setRetry(n=>n+1)} className="text-accent">{t('weather.retry')}</TapTarget></div>:loading?<p>{t('weather.loading')}</p>:data&&<>
      {data.hours.length===0&&<p className="text-sm text-text-muted mb-4">{t(data.gaps.some(g=>g.reason==='pending_weather')?'weather.pending':'weather.dayEmpty')}</p>}
      {data.hours.length>0&&<div className="grid lg:grid-cols-2 gap-6 mb-6">
        {[{id:'temperature',title:t('weather.temperature'),unit:'°C',lines:[['temperature',t('weather.temperature'),'var(--accent)'],['apparent',t('weather.apparent'),'var(--text-muted)']]},{id:'pressure',title:t('weather.pressure'),unit:'hPa',lines:[['pressure',t('weather.pressure'),'var(--accent)'],['seaPressure',t('weather.seaPressure'),'var(--text-muted)']]}].map(chart=><section key={chart.id} className="hv-card min-w-0" data-testid={`weather-${chart.id}-chart`}>
          <h2 className="text-base font-semibold mb-4">{chart.title} · {chart.unit}</h2>
          <div className="touch-pan-y" onTouchStart={replayTouchAsMove}><ResponsiveContainer width="100%" height={230}><LineChart data={data.points}>
            <CartesianGrid stroke="var(--border)" strokeDasharray="3 3"/>
            <XAxis dataKey="time" type="number" scale="time" domain={[range!.day.from,range!.day.to]} allowDataOverflow tickFormatter={time} tick={{fill:'var(--text-muted)',fontSize:11}} minTickGap={24}/>
            <YAxis domain={['auto','auto']} tick={{fill:'var(--text-muted)',fontSize:11}} width={48}/>
            <Tooltip labelFormatter={v=>time(Number(v))} formatter={v=>`${number(Number(v))} ${chart.unit}`} {...(coarse?{position:{y:0}}:{})}/>
            {chart.lines.map(([key,label,color],i)=><Line key={key} dataKey={key} name={label} type="linear" stroke={color} strokeDasharray={i?'4 3':undefined} dot={{r:2}} strokeWidth={2} connectNulls={false} isAnimationActive={false}/>)}
          </LineChart></ResponsiveContainer></div>
          <p className="text-[11px] text-text-muted">{chart.lines.map(l=>l[1]).join(' · ')}</p>
        </section>)}
      </div>}
      <section className="hv-card mb-6" data-testid="weather-gaps"><h2 className="text-base font-semibold mb-4">{t('weather.gaps')}</h2>
        {data.gaps.length? <ul className="text-xs text-text-muted space-y-1">{data.gaps.map((g,i)=><li key={i}>{time(Date.parse(g.from))}–{time(Date.parse(g.to))} · {t(gapKeys[g.reason]||'weather.unknownGap')}</li>)}</ul>:<p className="text-xs text-text-muted">{t('weather.noGaps')}</p>}
      </section>
      {data.hours.length>0&&<details className="hv-card mb-6" data-testid="weather-hours"><summary className="cursor-pointer min-h-12 py-3 text-base font-semibold mb-4">{t('weather.hours')} · {data.hours.length}</summary><div className="space-y-2">{data.hours.map(h=><article key={h.hour} data-testid="weather-hour" className="border border-border rounded-xl px-4 py-3 text-xs">
        <div className="flex flex-wrap gap-x-4 gap-y-1 tabular-nums"><strong>{new Date(h.hour).toLocaleString(dateLocaleFor(language),{timeZone:zone,month:'short',day:'numeric',hour:'2-digit',minute:'2-digit',timeZoneName:'shortOffset'})}–{time(Date.parse(h.hour)+3600000)}</strong><span>{t('weather.temperature')} {number(h.temperature_c)} °C</span><span>{t('weather.apparent')} {number(h.apparent_temperature_c)} °C</span><span>{t('weather.humidity')} {number(h.relative_humidity_percent)} %</span><span>{t('weather.wind')} {number(h.wind_speed_kmh)} km/h</span><span>{t('weather.precipitation')} {number(h.precipitation_mm)} mm</span><span>{t('weather.pressure')} {number(h.surface_pressure_hpa)} hPa</span><span>{t('weather.seaPressure')} {number(h.mean_sea_level_pressure_hpa)} hPa</span></div>
        <details className="mt-2 text-text-muted"><summary className="cursor-pointer min-h-8">{t('weather.provenance')}</summary><p>{h.source} · {h.model} · {h.latitude.toFixed(1)}, {h.longitude.toFixed(1)}</p><p>{t('weather.fetched')}: {new Date(h.fetched_at).toLocaleString(dateLocaleFor(language),{timeZone:zone})}</p></details>
      </article>)}</div></details>}
    </>}
    <details className="hv-card text-sm text-text-muted mt-6"><summary className="cursor-pointer min-h-8">{t('weather.provenance')}</summary><p>{t('weather.modelNotice')}</p></details>
  </main></AuthenticatedShell>;
}
