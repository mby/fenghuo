package store

// Legacy rows have unknown counts: never invent a packet denominator for them.
// Only groups made entirely of counted samples can be weighted exactly.
const probeAggregateSQL = `
CASE WHEN COUNT(samples)=COUNT(*) THEN COALESCE(SUM(sum_ms)/NULLIF(SUM(successes),0),-1)
 ELSE COALESCE(AVG(CASE WHEN avg_ms>=0 THEN avg_ms END),-1) END,
CASE WHEN COUNT(samples)=COUNT(*) THEN 100.0*(SUM(samples)-SUM(successes))/SUM(samples)
 ELSE AVG(loss) END,
CASE WHEN COUNT(samples)=COUNT(*) THEN SUM(samples) END,
CASE WHEN COUNT(samples)=COUNT(*) THEN SUM(successes) END`
