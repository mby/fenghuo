import { Area, AreaChart, CartesianGrid, Line, LineChart, XAxis, YAxis } from "recharts";
import { type ChartConfig, ChartContainer, ChartTooltip } from "@/components/ui/chart";
import { clock } from "@/lib/format";
import { useI18n } from "@/lib/i18n";

export interface Series {
  key: string;
  label: string;
  color: string;
}

type Row = { t: number } & Record<string, number | null>;

interface Props {
  title: string;
  data: Row[];
  series: Series[];
  format: (v: number) => string;
  domain?: [number, number];
  kind?: "area" | "line";
  /** "live" shows seconds on the axis, "long" shows dates. */
  scale?: "live" | "short" | "long";
  aside?: string;
}

export function MetricChart({ title, data, series, format, domain, kind = "area", scale = "short", aside }: Props) {
  const { t, lang } = useI18n();
  const config = Object.fromEntries(series.map((s) => [s.key, { label: s.label, color: s.color }])) as ChartConfig;
  const tick = (v: number) => clock(v, lang, scale === "long", scale === "live");
  // A fixed range gets evenly spaced ticks (0/25/50/75/100%) instead of Recharts' "nice" guesses.
  const ticks = domain ? [0, 1, 2, 3, 4].map((i) => domain[0] + ((domain[1] - domain[0]) * i) / 4) : undefined;

  const axes = (
    <>
      <CartesianGrid vertical={false} strokeDasharray="3 3" />
      <XAxis
        dataKey="t"
        type="number"
        domain={["dataMin", "dataMax"]}
        scale="time"
        tickFormatter={tick}
        tickLine={false}
        axisLine={false}
        minTickGap={48}
        tickMargin={6}
      />
      <YAxis
        domain={domain ?? [0, "auto"]}
        ticks={ticks}
        tickFormatter={format}
        tickLine={false}
        axisLine={false}
        width={76}
        tickCount={ticks ? undefined : 4}
      />
      <ChartTooltip
        cursor={{ strokeDasharray: "3 3" }}
        content={({ active, payload }) => {
          if (!active || !payload?.length) return null;
          const row = payload[0].payload as Row;
          return (
            <div className="grid min-w-36 gap-1.5 rounded-lg border bg-popover px-2.5 py-2 text-xs text-popover-foreground shadow-lg">
              <div className="font-medium">{clock(row.t, lang, true, scale === "live")}</div>
              {series.map((s) => (
                <div key={s.key} className="flex items-center justify-between gap-4">
                  <span className="flex items-center gap-1.5 text-muted-foreground">
                    <span className="size-2 rounded-[2px]" style={{ background: s.color }} />
                    {s.label}
                  </span>
                  <span className="tabular-nums">{row[s.key] == null ? "-" : format(row[s.key] as number)}</span>
                </div>
              ))}
            </div>
          );
        }}
      />
    </>
  );

  return (
    <section className="rounded-xl border bg-card p-4">
      <header className="mb-3 flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
        <h3 className="text-sm font-medium">{title}</h3>
        <div className="flex items-center gap-3 text-xs text-muted-foreground">
          {series.length > 1 &&
            series.map((s) => (
              <span key={s.key} className="flex items-center gap-1.5">
                <span className="size-2 rounded-[2px]" style={{ background: s.color }} aria-hidden />
                {s.label}
              </span>
            ))}
          {aside && <span className="tabular-nums">{aside}</span>}
        </div>
      </header>
      {data.length === 0 ? (
        <p className="flex h-44 items-center justify-center text-sm text-muted-foreground">{t("chart.empty")}</p>
      ) : (
        <ChartContainer config={config} className="aspect-auto h-44 w-full">
          {kind === "area" ? (
            <AreaChart data={data} margin={{ left: 0, right: 4, top: 4, bottom: 0 }}>
              {axes}
              {series.map((s) => (
                <Area
                  key={s.key}
                  dataKey={s.key}
                  type="monotone"
                  stroke={s.color}
                  fill={s.color}
                  fillOpacity={0.12}
                  strokeWidth={1.5}
                  isAnimationActive={false}
                  connectNulls={false}
                />
              ))}
            </AreaChart>
          ) : (
            <LineChart data={data} margin={{ left: 0, right: 4, top: 4, bottom: 0 }}>
              {axes}
              {series.map((s) => (
                <Line
                  key={s.key}
                  dataKey={s.key}
                  type="monotone"
                  stroke={s.color}
                  strokeWidth={1.5}
                  dot={data.length === 1}
                  isAnimationActive={false}
                  connectNulls={false}
                />
              ))}
            </LineChart>
          )}
        </ChartContainer>
      )}
    </section>
  );
}
